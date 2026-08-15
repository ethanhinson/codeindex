"""php-excimer (optional): drives the concurrently-built Docker Excimer lab
at bench/selfheal/php/. Skips cleanly when the lab is absent, docker is
unavailable, or the image fails to build. run.sh profiles inside the
container and lands a spool in app/.codeindex/runtime/; verify.sh does
build + ingest + search on the host and asserts observed evidence."""

import os
import shutil
import subprocess

from . import Scenario, SkipScenario, StepError


class PhpExcimerScenario(Scenario):
    name = "php-excimer"
    optional = True
    ladder = ["r3"]  # r1 (window) and r2 (path aliasing) aren't exposed by the lab

    def __init__(self, ctx):
        super().__init__(ctx)
        self.lab = os.path.join(ctx["selfheal_dir"], "php")
        self.app = os.path.join(self.lab, "app")
        self.verify_rc = None
        self.verify_out = ""

    def prepare(self):
        run_sh = os.path.join(self.lab, "run.sh")
        if not (os.path.isfile(run_sh) and os.path.isfile(os.path.join(self.lab, "verify.sh"))):
            raise SkipScenario("php lab not present (bench/selfheal/php/run.sh missing)")
        if shutil.which("docker") is None:
            raise SkipScenario("docker not available")
        rc, _ = self.run(["docker", "info"], timeout=30)
        if rc != 0:
            raise SkipScenario("docker daemon not running")
        rc, out = self.run(
            ["docker", "build", "-t", "codeindex-php-lab", "."],
            cwd=self.lab, timeout=600,
        )
        if rc != 0:
            raise SkipScenario("php lab image failed to build:\n" + out[-800:])

    def profile(self, window_mult=1):
        rc, out = self.run(["bash", os.path.join(self.lab, "run.sh")], cwd=self.lab, timeout=600)
        self.ctx["log"]("    " + out.strip().replace("\n", "\n    "))
        if rc != 0:
            raise StepError("php run.sh failed (rc=%d):\n%s" % (rc, out[-1500:]))

    def pipeline(self, resolved=False, rebuild=None):
        if rebuild:
            self.wipe_index(os.path.join(self.app, ".codeindex"), rebuild)
        rc, out = self.run(
            ["bash", os.path.join(self.lab, "verify.sh")],
            cwd=self.lab,
            env={"CODEINDEX_BIN": self.ctx["bin"]},
            timeout=600,
        )
        self.verify_rc, self.verify_out = rc, out
        self.ctx["log"]("    " + out.strip().replace("\n", "\n    "))
        self.record_ingest(out)

    def verify(self):
        # verify.sh itself asserts >=1 observed edge and the observed search
        # marker; its exit code is the assertion. edges/resolution are parsed
        # from its ingest output purely for reporting.
        return {"verify_sh_pass": self.verify_rc == 0}

    def spool_files(self):
        d = os.path.join(self.app, ".codeindex", "runtime")
        try:
            return sorted(
                os.path.join(d, f) for f in os.listdir(d) if f.endswith(".cxprof.jsonl")
            )
        except OSError:
            return []
