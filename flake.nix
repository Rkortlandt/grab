{
  description = "A terminal file manager";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    utils.url = "github:numtide/flake-utils";
  };
  outputs = { self, nixpkgs, utils }:
  utils.lib.eachDefaultSystem (system:
    let
      pkgs = import nixpkgs { inherit system; };
      
      # Define the package as a local variable first
      grab-pkg = pkgs.buildGoModule {
        pname = "grab";
        version = "0.1.1";
        src = ./.;
        vendorHash = null; 
      };
    in
    {
      packages.default = grab-pkg;

      devShells.default = pkgs.mkShell {
        # use nativeBuildInputs for binaries/compilers
        nativeBuildInputs = [ 
          pkgs.go 
          grab-pkg # Reference the local variable directly
        ];
        
        shellHook = ''
          pkill grab || true
          rm /tmp/grab.sock || true
          echo "Fresh GRAB environment clear"
          echo "⚡ GRAB Development Shell Active"
          echo "Usage: grab <file> | cb paste"
        '';
      };
    });
}
