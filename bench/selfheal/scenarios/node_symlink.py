"""node-symlink: identical app to node-registry, but the SDK is pointed at
the app THROUGH a symlink (/tmp/selfheal-link) while the pipeline is given
the REAL directory. This is the historical frame re-rooting trap (emitters
send whatever alias their runtime saw); ingest symlink-resolves both sides,
so it should pass — and if it regresses, it exercises the ladder (r2 is the
targeted fix)."""

import os

from .node_registry import NodeRegistryScenario

LINK = "/tmp/selfheal-link"


class NodeSymlinkScenario(NodeRegistryScenario):
    name = "node-symlink"

    def __init__(self, ctx):
        super().__init__(ctx)
        self.app = "/tmp/selfheal-apps/node-symlink-real"
        self.sdk_repo_path = LINK       # SDK sees the symlink alias
        self.pipeline_path = self.app   # pipeline sees the real dir

    def prepare(self):
        super().prepare()
        if os.path.islink(LINK) or os.path.exists(LINK):
            os.remove(LINK)
        os.symlink(self.app, LINK)
