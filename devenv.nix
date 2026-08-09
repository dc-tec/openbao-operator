{
  inputs,
  lib,
  pkgs,
  ...
}:

let
  readVersionFile = path: lib.removeSuffix "\n" (builtins.readFile path);

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

  # Build Helm 3 with the primary package set instead of mixing Nixpkgs stdenvs.
  helmFromPinnedDefinition = pkgs.callPackage (
    inputs.nixpkgs-helm3 + "/pkgs/applications/networking/cluster/helm"
  ) { };
  helmPackage =
    if lib.hasPrefix "3." (lib.getVersion helmFromPinnedDefinition) then
      helmFromPinnedDefinition
    else
      throw "Helm 3 is required, nixpkgs-helm3 provides ${lib.getVersion helmFromPinnedDefinition}";
in
{
  name = "openbao-operator";

  languages.go = {
    enable = true;
    version = goVersion;
    delve.enable = false;
    lsp.enable = false;
  };

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
    pkgs.jq
    pkgs.kind
    pkgs.kubectl
    pkgs.gnumake
    pkgs.python3
    pkgs.tilt
    pkgs.trivy
    helmPackage
    (exactPackage "Hugo" hugoVersion pkgs.hugo)
  ];

  enterShell = ''
    export GOPATH="''${XDG_CACHE_HOME:-$HOME/.cache}/openbao-operator/go"
    export PATH="$DEVENV_ROOT/bin:$GOPATH/bin:$PATH"
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
