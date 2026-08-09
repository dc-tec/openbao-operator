{
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
    GOROOT = "${goPackage}/share/go";
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

  tasks = {
    "operator:verify-toolchain" = {
      exec = "make verify-devenv";
      before = [ "devenv:enterTest" ];
    };

    "operator:setup".exec = "make bootstrap";
    "operator:doctor".exec = "make doctor";
    "operator:ci-core".exec = "make ci-core";
  };
}
