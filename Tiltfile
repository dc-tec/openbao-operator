config.define_string("operator-version")
config.define_string("init-image-repository")
config.define_string("backup-image-repository")
config.define_string("upgrade-image-repository")

cfg = config.parse()

operator_version = cfg.get("operator-version", "edge")
init_image_repository = cfg.get("init-image-repository", "ghcr.io/dc-tec/openbao-init")
backup_image_repository = cfg.get("backup-image-repository", "ghcr.io/dc-tec/openbao-backup")
upgrade_image_repository = cfg.get("upgrade-image-repository", "ghcr.io/dc-tec/openbao-upgrade")

context = k8s_context()
local_prefixes = [
    "kind-",
    "k3d-",
    "minikube",
    "docker-desktop",
    "rancher-desktop",
    "colima",
]

local_context_ok = False
for prefix in local_prefixes:
    if context == prefix or context.startswith(prefix):
        local_context_ok = True

if not local_context_ok:
    fail("Tilt is intended for a local dev cluster. Current context: %s" % context)

allow_k8s_contexts(context)

watch_settings(ignore=[
    ".git",
    ".venv",
    ".venv-docs",
    "bin",
    "dist",
    "site",
    "tmp",
    "cover.out",
    "coverage.out",
    "report.xml",
])

render_env = {
    "OPERATOR_VERSION": operator_version,
    "OPERATOR_INIT_IMAGE_REPOSITORY": init_image_repository,
    "OPERATOR_BACKUP_IMAGE_REPOSITORY": backup_image_repository,
    "OPERATOR_UPGRADE_IMAGE_REPOSITORY": upgrade_image_repository,
    "TILT_MANAGER_IMAGE": "controller",
}

k8s_yaml(local("bash hack/dev/render-tilt-manifests.sh", env=render_env, quiet=True))

docker_build(
    "controller",
    ".",
    dockerfile="Dockerfile",
    ignore=[
        ".git",
        ".venv",
        ".venv-docs",
        "bin",
        "dist",
        "site",
        "tmp",
        "cover.out",
        "coverage.out",
        "report.xml",
    ],
)

k8s_resource("openbao-operator-controller", port_forwards=["8081:8081"])
k8s_resource("openbao-operator-provisioner", port_forwards=["18081:8081"])

go_deps = [
    "api",
    "cmd",
    "internal",
    "test",
    "go.mod",
    "go.sum",
    "Makefile",
    "vendor",
]

local_resource(
    "test-sum",
    "make test-sum",
    deps=go_deps,
    trigger_mode=TRIGGER_MODE_MANUAL,
    auto_init=False,
)

local_resource(
    "test-integration-sum",
    "make test-integration-sum",
    deps=go_deps + ["config"],
    trigger_mode=TRIGGER_MODE_MANUAL,
    auto_init=False,
)

local_resource(
    "verify-generated",
    "make verify-generated verify-helm",
    deps=["api", "charts", "config", "docs", "hack", "Makefile", "go.mod", "go.sum"],
    trigger_mode=TRIGGER_MODE_MANUAL,
    auto_init=False,
)
