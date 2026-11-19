self:
{ config, lib, pkgs, ... }:

let
  cfg = config.services.gymbag;
  effectiveConfig =
    if cfg.configFile != null then cfg.configFile
    else if cfg.configText != null then pkgs.writeText "Gymbagfile" cfg.configText
    else null;
in
{
  options.services.gymbag = {
    enable = lib.mkEnableOption "Gymbag HTTPS reverse proxy and Proof-of-Work WAF";

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
      defaultText = lib.literalExpression "inputs.gymbag.packages.\${pkgs.stdenv.hostPlatform.system}.default";
      description = "The gymbag package to run.";
    };

    configFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = "Path to a Gymbagfile, Caddyfile, or JSON configuration file.";
    };

    configText = lib.mkOption {
      type = lib.types.nullOr lib.types.lines;
      default = null;
      example = ''
        app.example.com {
            reverse_proxy http://127.0.0.1:3000
            verify 3
        }
      '';
      description = "Inline Gymbagfile configuration text (ignored if configFile is set).";
    };

    watch = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = "Watch configFile for changes and hot-reload site policies.";
    };

    environmentFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = "Optional environment file containing GYMBAG_* overrides or secrets.";
    };

    openFirewall = lib.mkOption {
      type = lib.types.bool;
      default = false;
      description = "Whether to open httpPort and httpsPort in networking.firewall.";
    };

    httpPort = lib.mkOption {
      type = lib.types.port;
      default = 80;
      description = "HTTP redirect / ACME HTTP-01 port to open when openFirewall is enabled.";
    };

    httpsPort = lib.mkOption {
      type = lib.types.port;
      default = 443;
      description = "HTTPS gateway port to open when openFirewall is enabled.";
    };

    extraArgs = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ ];
      description = "Additional CLI arguments passed to `gymbag run`.";
    };
  };

  config = lib.mkIf cfg.enable {
    assertions = [
      {
        assertion = effectiveConfig != null || cfg.extraArgs != [ ];
        message = "services.gymbag requires setting configFile, configText, or extraArgs.";
      }
    ];

    networking.firewall.allowedTCPPorts = lib.mkIf cfg.openFirewall [
      cfg.httpPort
      cfg.httpsPort
    ];

    systemd.services.gymbag = {
      description = "Gymbag HTTPS Reverse Proxy & Proof-of-Work WAF";
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];
      wantedBy = [ "multi-user.target" ];

      serviceConfig = {
        Type = "simple";
        DynamicUser = true;
        StateDirectory = "gymbag";
        WorkingDirectory = "/var/lib/gymbag";
        Environment = [ "GYMBAG_CERT_DIR=/var/lib/gymbag/certs" ];
        EnvironmentFile = lib.mkIf (cfg.environmentFile != null) cfg.environmentFile;
        ExecStart = lib.concatStringsSep " " (
          [ "${lib.getExe cfg.package}" "run" ]
          ++ lib.optionals (effectiveConfig != null) [ "--config" "${effectiveConfig}" ]
          ++ lib.optional (cfg.configFile != null && cfg.watch) "--watch"
          ++ map lib.escapeShellArg cfg.extraArgs
        );
        Restart = "on-failure";
        RestartSec = "5s";
        LimitNOFILE = 65536;
        AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];
        CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
        NoNewPrivileges = true;
      };
    };
  };
}
