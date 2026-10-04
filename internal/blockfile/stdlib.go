package blockfile

import "strings"

// stdlib holds the public top-level module names of the Python 3.10–3.13 standard library (F012):
// the union of sys.stdlib_module_names from CPython 3.11, 3.12 and 3.13, plus binhex (3.10) and
// __future__. Read-only.
var stdlib = func() map[string]bool {
	set := map[string]bool{}
	for _, m := range strings.Fields(`__future__
abc aifc antigravity argparse array ast asynchat asyncio asyncore atexit audioop base64 bdb binascii
binhex bisect builtins bz2 calendar cgi cgitb chunk cmath cmd code codecs codeop collections colorsys
compileall concurrent configparser contextlib contextvars copy copyreg cProfile crypt csv ctypes
curses dataclasses datetime dbm decimal difflib dis distutils doctest email encodings ensurepip enum
errno faulthandler fcntl filecmp fileinput fnmatch fractions ftplib functools gc genericpath getopt
getpass gettext glob graphlib grp gzip hashlib heapq hmac html http idlelib imaplib imghdr imp
importlib inspect io ipaddress itertools json keyword lib2to3 linecache locale logging lzma mailbox
mailcap marshal math mimetypes mmap modulefinder msilib msvcrt multiprocessing netrc nis nntplib nt
ntpath nturl2path numbers opcode operator optparse os ossaudiodev pathlib pdb pickle pickletools pipes
pkgutil platform plistlib poplib posix posixpath pprint profile pstats pty pwd py_compile pyclbr pydoc
pydoc_data pyexpat queue quopri random re readline reprlib resource rlcompleter runpy sched secrets
select selectors shelve shlex shutil signal site smtpd smtplib sndhdr socket socketserver spwd sqlite3
sre_compile sre_constants sre_parse ssl stat statistics string stringprep struct subprocess sunau
symtable sys sysconfig syslog tabnanny tarfile telnetlib tempfile termios textwrap this threading time
timeit tkinter token tokenize tomllib trace traceback tracemalloc tty turtle turtledemo types typing
unicodedata unittest urllib uu uuid venv warnings wave weakref webbrowser winreg winsound wsgiref
xdrlib xml xmlrpc zipapp zipfile zipimport zlib zoneinfo`) {
		set[m] = true
	}
	return set
}()
