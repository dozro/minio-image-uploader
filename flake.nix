{
  description = "Minio Image Uploader";
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };
  outputs = { self, nixpkgs, flake-utils, ... }:
    flake-utils.lib.eachDefaultSystem (system: let
      pkgs = import nixpkgs { inherit system; };
    in {
      packages.default = pkgs.buildGoModule {
        pname = "minio-image-uploader";
        version = "0.1.0";
        src = ./.;
        vendorHash = null; # Replace 'null' with a hash
      };
      apps.default = {
        type = "app";
        program = "${self.packages.${system}.default}/bin/my-go-program";
      };
    });
}

