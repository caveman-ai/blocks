package capture

// callNames is the fixed call-name table of the shape fingerprint (docs/HOOK.md, "Two identities per
// body"). It is data: changing it changes every fp and is a golden-table change.
var callNames = []string{
	"json.load", "json.loads", "json.dump", "json.dumps", "open", "print", "re.sub", "re.findall",
	"re.search", "Counter", "glob", "os.walk", "subprocess", "sys.argv", "argparse", "urllib",
	"requests", "csv", "yaml", "pathlib", "sqlite3", "psycopg", "time.sleep",
}
