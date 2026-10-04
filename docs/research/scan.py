"""Scan local Claude Code transcripts: what do agents write and run in code mode?"""
import json, re, os, sys, hashlib, collections, glob
root = os.path.expanduser('~/.claude/projects')
files = glob.glob(root + '/**/*.jsonl', recursive=True)
C = collections.Counter
stats = C(); kinds = C(); first_tok = C(); heredoc_lang = C(); write_ext = C()
out_sizes = []; cluster = collections.defaultdict(lambda: {'n':0,'sessions':set(),'ex':None,'kind':None})
written_files = {}  # path -> session ; later executed?
exec_after_write = C()
examples = collections.defaultdict(list)
norm_re = [(re.compile(r'"[^"]*"'), '"S"'), (re.compile(r"'[^']*'"), "'S'"), (re.compile(r'\b[0-9a-f]{7,40}\b'), 'HASH'),
           (re.compile(r'/[\w./-]+'), '/P'), (re.compile(r'\b\d+\b'), 'N'), (re.compile(r'\s+'), ' ')]
def norm(s):
    for r, rep in norm_re: s = r.sub(rep, s)
    return s.strip()
def classify(cmd):
    c = cmd.strip()
    if re.search(r"<<\s*-?\s*['\"]?(EOF|PY|SH|JS|SQL|'EOF')", c) or re.search(r'\b(python3?|node|bun|deno)\s+-\s*<<', c): return 'heredoc'
    if re.search(r'\b(python3?|node|bun|deno)\s+-[ce]\s', c): return 'inline-script'
    if re.search(r'\b(for|while)\s+\w*.*\bdo\b', c) or re.search(r'\bif\s+\[', c): return 'shell-loop'
    seps = c.count('&&') + c.count('||') + c.count(';') + c.count('|')
    if '\n' in c and len(c) > 200: return 'multiline'
    if seps >= 3: return 'pipeline'
    if seps >= 1: return 'compound'
    return 'single'
def heredoc_kind(cmd):
    m = re.search(r'\b(python3?|node|bun|deno|psql|clickhouse-client|sqlite3|bash|sh|cat)\b[^\n]*<<', cmd)
    return m.group(1) if m else 'other'
pending = {}  # tool_use_id -> (kind, len)
for i, f in enumerate(files):
    sess = os.path.basename(f)
    try: fh = open(f, errors='ignore')
    except: continue
    for line in fh:
        if '"tool_use"' not in line and '"tool_result"' not in line: continue
        try: o = json.loads(line)
        except: continue
        m = o.get('message') or {}
        content = m.get('content') if isinstance(m, dict) else None
        if not isinstance(content, list): continue
        for c in content:
            if not isinstance(c, dict): continue
            if c.get('type') == 'tool_use':
                name = c.get('name'); inp = c.get('input') or {}
                if name == 'Bash':
                    cmd = inp.get('command') or ''
                    if not cmd: continue
                    k = classify(cmd); kinds[k] += 1; stats['bash'] += 1
                    pending[c.get('id')] = k
                    ft = re.match(r'\s*(?:cd\s+\S+\s*(?:&&|;)\s*)?(\S+)', cmd)
                    if ft: first_tok[ft.group(1).split('/')[-1]] += 1
                    if k == 'heredoc': heredoc_lang[heredoc_kind(cmd)] += 1
                    if k in ('heredoc','inline-script','shell-loop','multiline','pipeline'):
                        h = hashlib.md5(norm(cmd)[:800].encode()).hexdigest()[:10]
                        e = cluster[h]; e['n'] += 1; e['sessions'].add(sess); e['kind'] = k
                        if e['ex'] is None: e['ex'] = cmd[:400]
                    for p in written_files:
                        if p in cmd and written_files[p] == sess: exec_after_write[p.rsplit('.',1)[-1] if '.' in p else 'noext'] += 1; written_files[p] = None
                elif name == 'Write':
                    p = inp.get('file_path') or ''
                    ext = p.rsplit('.',1)[-1] if '.' in p else 'noext'
                    if ext in ('py','sh','mjs','js','ts','sql','bash') or '/tmp/' in p:
                        write_ext[ext] += 1; stats['write-script'] += 1
                        written_files[p] = sess
                        if len(examples[ext]) < 8: examples[ext].append(p)
            elif c.get('type') == 'tool_result' and c.get('tool_use_id') in pending:
                k = pending.pop(c['tool_use_id'])
                cc = c.get('content'); 
                if isinstance(cc, list): cc = ''.join(x.get('text','') for x in cc if isinstance(x, dict))
                out_sizes.append((k, len(cc or '')))
    if i % 500 == 0: print('..', i, file=sys.stderr)
print('files', len(files)); print(dict(stats)); print('kinds', kinds.most_common())
print('first_tok', first_tok.most_common(45))
print('heredoc_lang', heredoc_lang.most_common())
print('write_ext', write_ext.most_common()); print('exec_after_write', exec_after_write.most_common())
print('write examples', {k: v[:5] for k, v in examples.items()})
import statistics
by = collections.defaultdict(list)
for k, n in out_sizes: by[k].append(n)
print('output chars by kind (n, median, p90, share>2KB):')
for k, v in by.items():
    v.sort(); print(' ', k, len(v), v[len(v)//2], v[int(len(v)*.9)], round(sum(1 for x in v if x > 2000)/len(v), 2))
print('total bash output MB', round(sum(n for _, n in out_sizes)/1e6, 1))
rep = [(h, e) for h, e in cluster.items() if len(e['sessions']) >= 3]
rep.sort(key=lambda x: -x[1]['n'])
print('clusters total', len(cluster), 'repeating across>=3 sessions', len(rep), 'runs in repeating', sum(e['n'] for _, e in rep))
for h, e in rep[:40]: print('\n#', e['n'], 'runs', len(e['sessions']), 'sessions', e['kind'], '\n', e['ex'].replace('\n', '\\n')[:300])
