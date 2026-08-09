{
  config,
  lib,
  pkgs,
  ...
}:

let
  readVersionFile = path: lib.removeSuffix "\n" (builtins.readFile path);

  parseToolVersion = line:
    let
      match = builtins.match "([A-Z0-9_]+)=(v?[0-9].*)" line;
    in
    if match == null then
      null
    else
      {
        name = builtins.elemAt match 0;
        value = builtins.elemAt match 1;
      };
  toolVersions = builtins.listToAttrs (
    builtins.filter (entry: entry != null) (
      map parseToolVersion (lib.splitString "\n" (builtins.readFile ./hack/dev/tool-versions.env))
    )
  );
  toolVersion = name:
    if builtins.hasAttr name toolVersions then
      lib.removePrefix "v" toolVersions.${name}
    else
      throw "hack/dev/tool-versions.env does not declare ${name}";

  goLine =
    lib.findFirst (line: lib.hasPrefix "go " line) (throw "go.mod does not declare a Go version")
      (lib.splitString "\n" (builtins.readFile ./go.mod));
  goVersion = lib.removePrefix "go " goLine;
  hugoVersion = readVersionFile ./.hugo-version;

  exactPackage =
    name: expected: package:
    if lib.getVersion package == expected then
      package
    else
      throw "${name} version mismatch: expected ${expected}, nixpkgs provides ${lib.getVersion package}";

  goPackage = exactPackage "Go" goVersion pkgs.go;

in
{
  name = "openbao-operator";

  packages = [
    pkgs.bash
    pkgs.coreutils
    pkgs.curl
    pkgs.docker-client
    pkgs.findutils
    pkgs.gawk
    pkgs.git
    pkgs.gnugrep
    pkgs.gnused
    goPackage
    pkgs.jq
    (exactPackage "Kind" (toolVersion "KIND_VERSION") pkgs.kind)
    (exactPackage "kubectl" (toolVersion "KUBECTL_VERSION") pkgs.kubectl)
    pkgs.gnumake
    pkgs.python3
    (exactPackage "Tilt" (toolVersion "TILT_VERSION") pkgs.tilt)
    (exactPackage "Trivy" (toolVersion "TRIVY_VERSION") pkgs.trivy)
    (exactPackage "Helm" (toolVersion "HELM_VERSION") pkgs.kubernetes-helm)
    (exactPackage "Hugo" hugoVersion pkgs.hugo)
  ];

  env = {
    # Match the Go language module's normalized value when the editor profile is active.
    GOROOT = "${goPackage}/share/go/";
    GOTOOLCHAIN = "local";
  };

  profiles.editor.module = {
    languages.go = {
      enable = true;
      package = goPackage;
      delve.enable = true;
      lsp.enable = true;
    };
  };

  enterShell = ''
    export GOPATH="''${XDG_CACHE_HOME:-$HOME/.cache}/openbao-operator/go"
    export PATH="$DEVENV_PROFILE/bin:$DEVENV_ROOT/bin:$GOPATH/bin:$PATH"
  '';

  git-hooks.hooks = {
    openbao-operator-pre-commit = {
      enable = true;
      name = "OpenBao Operator pre-commit";
      entry = "${pkgs.bash}/bin/bash hack/dev/pre-commit.sh";
      stages = [ "pre-commit" ];
      pass_filenames = false;
      always_run = true;
    };

    openbao-operator-pre-push = {
      enable = true;
      name = "OpenBao Operator pre-push";
      entry = "${pkgs.bash}/bin/bash hack/dev/pre-push.sh";
      stages = [ "pre-push" ];
      pass_filenames = false;
      always_run = true;
    };
  };

  tasks = {
    "operator:migrate-legacy-git-hooks" = {
      description = "Remove the retired .githooks path before Devenv installs native hooks";
      exec = ''
        if test "$(git config --local --get core.hooksPath 2>/dev/null || true)" = ".githooks"; then
          git config --local --unset-all core.hooksPath
        fi
      '';
      status = ''
        test "$(git config --local --get core.hooksPath 2>/dev/null || true)" != ".githooks"
      '';
      cwd = config.git.root;
      before = [ "devenv:git-hooks:install" ];
    };

    "operator:verify-toolchain" = {
      description = "Verify the pinned service-independent toolchain contract";
      exec = "make verify-devenv";
      cwd = config.git.root;
      after = [ "devenv:git-hooks:install" ];
      before = [ "devenv:enterTest" ];
    };

    "operator:bootstrap" = {
      description = "Install repository-managed tools required by the core contributor workflow";
      exec = "make bootstrap";
      cwd = config.git.root;
    };

    "operator:doctor" = {
      description = "Check external runtime prerequisites such as Docker and Kubernetes access";
      exec = "make doctor";
      cwd = config.git.root;
    };

    "operator:ci-core" = {
      description = "Run the cluster-independent pull-request-equivalent gate";
      exec = "make ci-core";
      cwd = config.git.root;
    };
  };
}
