"""Pass 2: what do inline scripts (heredoc / python -c / node -e) actually do?"""
import json, re, os, glob, collections, random
root = os.path.expanduser('~/.claude/projects')
files = glob.glob(root + '/**/*.jsonl', recursive=True)
CATS = [
 ('edit-file-in-place', r'(open\([^)]*[\'"]w[\'"]\)|\.write\(|writeFileSync|re\.sub\(.*\n.*write|Path\([^)]*\)\.write_text)'),
 ('inspect-json', r'json\.load|JSON\.parse|require\([\'"][^\'"]*\.json'),
 ('poll-wait', r'\buntil\b|\bwhile\b.*sleep|time\.sleep'),
 ('run-tests-summarize', r'(go test|pytest|vitest|pnpm test|npm test|cargo test).*\|\s*(tail|grep|head)'),
 ('http-call', r'curl |requests\.(get|post)|urllib|fetch\('),
 ('db-query', r'psql|clickhouse|sqlite3|sqlite|SELECT\s'),
 ('stats-aggregate', r'Counter\(|statistics\.|mean\(|median|defaultdict|groupby|sum\(|\.reduce\('),
 ('text-extract-grep', r'\bre\.(findall|search|finditer)|readlines\(\)|splitlines\(\)|\.match\('),
 ('read-slice-file', r'open\([^)]*\)\.read\(\)\[|\[\d+:\d+\]|sed -n|\.slice\('),
 ('yaml-toml-parse', r'yaml\.|tomllib|toml\.'),
 ('ast-parse', r'\bast\.|tree_sitter|ts\.createSourceFile'),
 ('csv', r'\bcsv\.'),
 ('render-report', r'\.md[\'"]|markdown|tabulate|print\(f?["\']\|'),
]
cats = collections.Counter(); ex = collections.defaultdict(list); loc = []; shape = collections.Counter(); shape_ex = {}
total = 0
def shape_of(code):
    mods = sorted(set(re.findall(r'^\s*(?:import|from)\s+([\w.]+)', code, re.M)))
    calls = sorted(set(re.findall(r'\b(json\.load|json\.dump|open|print|re\.sub|re\.findall|Counter|glob|os\.walk|subprocess|sys\.argv|argparse|requests)\b', code)))
    return ','.join(mods) + ' | ' + ','.join(calls)
for f in files:
    try: fh = open(f, errors='ignore')
    except: continue
    for line in fh:
        if '"Bash"' not in line: continue
        try: o = json.loads(line)
        except: continue
        m = o.get('message') or {}
        content = m.get('content') if isinstance(m, dict) else None
        if not isinstance(content, list): continue
        for c in content:
            if not (isinstance(c, dict) and c.get('type') == 'tool_use' and c.get('name') == 'Bash'): continue
            cmd = (c.get('input') or {}).get('command') or ''
            if not (re.search(r'<<\s*-?\s*[\'"]?\w+', cmd) or re.search(r'\b(python3?|node|bun)\s+-[ce]\s', cmd)): continue
            if re.search(r'\bcat\s*>', cmd) and not re.search(r'\b(python3?|node|bun)\b', cmd): continue  # file writes via cat, not scripts
            total += 1
            loc.append(cmd.count('\n') + 1)
            hit = False
            for name, rx in CATS:
                if re.search(rx, cmd, re.S):
                    cats[name] += 1; hit = True
                    if len(ex[name]) < 3 and random.random() < 0.05: ex[name].append(cmd[:260].replace('\n', '\\n'))
            if not hit:
                cats['other'] += 1
                if len(ex['other']) < 6 and random.random() < 0.05: ex['other'].append(cmd[:260].replace('\n', '\\n'))
            if 'python' in cmd:
                s = shape_of(cmd); shape[s] += 1
                shape_ex.setdefault(s, cmd[:200].replace('\n', '\\n'))
            if 'sys.argv' in cmd or 'argparse' in cmd: cats['_takes-params'] += 1
            if re.search(r'json\.dumps\(|JSON\.stringify\(', cmd) and re.search(r'print\(json\.dumps|console\.log\(JSON\.stringify', cmd): cats['_emits-json'] += 1
            if re.search(r'\bdef \w+\(', cmd): cats['_defines-function'] += 1
loc.sort()
print('inline scripts', total, 'lines median', loc[len(loc)//2], 'p75', loc[int(len(loc)*.75)], 'p90', loc[int(len(loc)*.9)], 'share>=10 lines', round(sum(1 for x in loc if x >= 10)/len(loc), 2))
print('categories (overlapping):'); [print(f'  {k:24s} {v:6d}  {v/total:.0%}') for k, v in cats.most_common()]
print('\nexamples:'); 
for k, v in ex.items():
    for e in v: print(' ', k, '::', e[:230])
print('\ntop python shapes (imports | calls):')
for s, n in shape.most_common(25): print(f'  {n:5d}  {s[:90]:90s}  e.g. {shape_ex[s][:80]}')
