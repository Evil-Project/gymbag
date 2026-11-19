{
  description = "Gymbag — Single-binary HTTPS Reverse Proxy & Proof-of-Work WAF";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      supportedSystems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
    in
    {
      packages = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          version =
            if self ? shortRev then "0.1.0-${self.shortRev}"
            else "0.1.0-dirty";
          gymbag = pkgs.buildGoModule {
            pname = "gymbag";
            inherit version;
            src = self;
            vendorHash = null;
            subPackages = [ "cmd/gymbag" ];
            ldflags = [
              "-s"
              "-w"
              "-X gymbag/internal/cli.Version=${version}"
            ];
            env.CGO_ENABLED = "0";
            meta = with pkgs.lib; {
              description = "Single-binary HTTPS reverse proxy & Proof-of-Work WAF in pure Go";
              homepage = "https://github.com/Evil-Project/gymbag";
              license = licenses.asl20;
              mainProgram = "gymbag";
              platforms = platforms.unix;
            };
          };
        in
        {
          default = gymbag;
          inherit gymbag;
        });

      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/gymbag";
        };
      });

      devShells = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              go
              gopls
              git
            ];
          };
        });

      nixosModules = {
        default = import ./nix/module.nix self;
        gymbag = import ./nix/module.nix self;
      };
    };
}
