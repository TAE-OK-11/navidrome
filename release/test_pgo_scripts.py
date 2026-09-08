"""Fast build-orchestration regressions; no compiler or worker builds needed."""
import json
import os
import re
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent


class PgoScripts(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="navidrome-pgo-test-")
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        self.bin = self.path / "bin"
        self.bin.mkdir()
        self.trace = self.path / "trace.jsonl"
        self.env = dict(os.environ, PATH=f"{self.bin}:{os.environ['PATH']}", TRACE=str(self.trace))
        self.env.update(PGO_PROFILE_DIR=str(self.path / "go-profiles"),
                        PGO_OUTPUT=str(self.path / "default.pgo"),
                        PGO_BUILD_TAGS="netgo,sqlite_fts5", RUST_PGO_GO_ROUNDS="0",
                        RUST_PGO_DIR=str(self.path / "rust-profiles"),
                        CARGO_TARGET_DIR=str(self.path / "target"))
        for key in ("RUST_PGO_PROFDATA", "RUSTFLAGS", "CARGO_ENCODED_RUSTFLAGS"):
            self.env.pop(key, None)

    def tool(self, name, body):
        path = self.bin / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("#!/usr/bin/env python3\nimport os,sys,json\nfrom pathlib import Path\n" + body)
        path.chmod(0o755)
        return path

    def run_script(self, name):
        return subprocess.run(["sh", str(ROOT / "release" / name)], cwd=ROOT,
                              env=self.env, text=True, capture_output=True, timeout=30)

    def records(self):
        return [json.loads(line) for line in self.trace.read_text().splitlines()]

    def fake_go(self):
        self.tool("go", '''
args = sys.argv[1:]
with open(os.environ['TRACE'], 'a') as f: f.write(json.dumps(args)+'\\n')
if args[0] == 'test':
    for arg in args:
        if arg.startswith('-cpuprofile=') or arg.startswith('-o='):
            p=Path(arg.split('=',1)[1]); p.parent.mkdir(parents=True,exist_ok=True); p.write_text('profile')
    if os.environ.get('FAIL_BENCH'): sys.exit(17)
    if not os.environ.get('SKIP_BENCH'): print('BenchmarkWork-4  100  12.0 ns/op')
elif '-proto' in args:
    Path(next(a.split('=',1)[1] for a in args if a.startswith('-output='))).write_text('merged')
''')

    def test_go_training_uses_writable_scratch_and_disables_automatic_pgo(self):
        self.fake_go()
        result = self.run_script("pgo-train.sh")
        self.assertEqual(result.returncode, 0, result.stderr)
        runs = [r for r in self.records() if r[0] == "test"]
        self.assertEqual(len(runs), 20)
        for run in runs:
            self.assertIn("-pgo=off", run)
            output = Path(next(a.split("=", 1)[1] for a in run if a.startswith("-o=")))
            self.assertEqual(output.parent, self.path / "go-profiles")
            self.assertFalse(output.exists())
        self.assertTrue((self.path / "default.pgo").exists())

    def test_go_training_rejects_skipped_benchmark_even_with_nonempty_profile(self):
        self.fake_go()
        self.env["SKIP_BENCH"] = "1"
        result = self.run_script("pgo-train.sh")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("no completed benchmark", result.stderr)
        self.assertFalse((self.path / "default.pgo").exists())

    def test_go_training_propagates_failure_and_removes_test_binary(self):
        self.fake_go()
        self.env["FAIL_BENCH"] = "1"
        result = self.run_script("pgo-train.sh")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(list((self.path / "go-profiles").glob("*.test")))

    def fake_rust(self):
        sysroot = self.path / "sysroot"
        self.env['MOCK_SYSROOT'] = str(sysroot)
        self.tool("rustc", '''
print(os.environ['MOCK_SYSROOT'] if '--print' in sys.argv else 'host: x86_64-unknown-linux-gnu')
''')
        self.tool("cargo", '''
import re
with open(os.environ['TRACE'], 'a') as f:
    f.write(json.dumps({'args':sys.argv[1:], 'flags':os.environ.get('RUSTFLAGS','')})+'\\n')
if sys.argv[1]=='bench':
    directory=re.search(r'-Cprofile-generate=(\\S+)',os.environ['RUSTFLAGS']).group(1)
    Path(directory,Path.cwd().name+'.profraw').write_text('raw')
''')
        return self.tool(str(sysroot / 'lib/rustlib/x86_64-unknown-linux-gnu/bin/llvm-profdata'), '''
Path(sys.argv[sys.argv.index('-o')+1]).write_text('merged')
''')

    def test_rust_uses_matching_llvm_and_target_isolates_build_scripts(self):
        self.fake_rust()
        self.tool("llvm-profdata", "sys.exit('must not use unrelated system LLVM')\n")
        result = self.run_script("rust-pgo-train.sh")
        self.assertEqual(result.returncode, 0, result.stderr)
        runs = self.records()
        self.assertEqual(len(runs), 6)
        for run in runs:
            args = run['args']
            self.assertEqual(args[args.index('--target')+1], 'x86_64-unknown-linux-gnu')
        self.assertIn('-Cprofile-use=', runs[-1]['flags'])
        self.assertIn('/x86_64-unknown-linux-gnu/release-fat/', result.stdout)

    def test_docker_native_and_runtime_failures_are_not_hidden(self):
        self.tool("make", "sys.exit(17)\n")
        self.tool("apt-get", "sys.exit(17)\n")
        text = (ROOT / "Dockerfile.jbs").read_text()
        instructions = re.findall(r'^RUN (?:[^\n]*\\\n)*[^\n]*', text, re.MULTILINE)
        selected = [run for run in instructions if 'ccache -s' in run or
                    ('apt-get install' in run and "__pycache__" in run)]
        self.assertEqual(len(selected), 4)
        for run in selected:
            command = re.sub(r'--mount=\S+\s*', '', run[4:].replace('\\\n', ' '))
            result = subprocess.run(['sh', '-c', command], cwd=self.path,
                                    env=self.env, capture_output=True, timeout=5)
            self.assertNotEqual(result.returncode, 0, run)

    def test_rust_fails_before_build_when_matching_profdata_is_missing(self):
        self.fake_rust().unlink()
        result = self.run_script("rust-pgo-train.sh")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('llvm-tools-preview', result.stderr)
        self.assertFalse(self.trace.exists())


if __name__ == '__main__':
    unittest.main()
