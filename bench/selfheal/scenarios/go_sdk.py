"""go-sdk: cheap liveness scenario — the Go SDK's own e2e test profiles the
test binary, spools cxprof, and asserts conformance + resolution. No temp
app, no ladder rungs apply (the test is hermetic); persistent failure goes
straight to quarantine (which has no spool to move) and fails the harness."""

from . import Scenario, StepError


class GoSdkScenario(Scenario):
    name = "go-sdk"
    ladder = []  # r1-r3 target our spool/pipeline; none apply to `go test`

    def __init__(self, ctx):
        super().__init__(ctx)
        self.test_rc = None
        self.test_out = ""

    def prepare(self):
        pass

    def profile(self, window_mult=1):
        pass  # the go test does its own profiling

    def pipeline(self, resolved=False, rebuild=None):
        rc, out = self.run(
            ["go", "test", "./sdk/go/agent/", "-run", "TestSpoolConformsAndResolves", "-count=1"],
            cwd=self.ctx["repo_root"],
            timeout=300,
        )
        self.test_rc, self.test_out = rc, out
        self.ctx["log"]("    " + out.strip().replace("\n", "\n    "))
        if rc != 0:
            raise StepError("go test failed (rc=%d):\n%s" % (rc, out))

    def verify(self):
        return {"go_test_pass": self.test_rc == 0}
