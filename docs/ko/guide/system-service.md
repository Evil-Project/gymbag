# 시스템 서비스 설정

`gymbag service`는 네 가지 init 시스템에서 OS 백그라운드 서비스를 생성, 등록, 시작, 중지, 재시작, 상태 확인, 제거합니다.

| 관리자 | 대상 OS / Init | 기본 유닛 경로 |
| :--- | :--- | :--- |
| `systemd` | Linux (Ubuntu, Debian, Fedora, Arch, RHEL) | `/etc/systemd/system/<name>.service` (`--user` 시 `~/.config/systemd/user/<name>.service`) |
| `launchd` | macOS (LaunchDaemons / LaunchAgents) | `/Library/LaunchDaemons/io.gymbag.<name>.plist` (`--user` 시 `~/Library/LaunchAgents/io.gymbag.<name>.plist`) |
| `openrc` | Linux (Alpine, Gentoo, Artix) | `/etc/init.d/<name>` |
| `supervisor` | Unix / 컨테이너 (`supervisord`) | `/etc/supervisor/conf.d/<name>.conf` (`--user` 시 `~/.config/supervisor/conf.d/<name>.conf`) |

## 두 줄 서비스 등록

`gymbag.json`이나 `Gymbagfile`이 준비되었으면 다음 순서로 등록하고 실행합니다.

```bash
# 1. 서비스 등록 (systemd / launchd / openrc / supervisor 자동 감지)
sudo ./gymbag service install --config /etc/gymbag/gymbag.json

# 2. 시작
sudo ./gymbag service start
```

설정 생성과 서비스 등록을 한 번에 처리할 수도 있습니다.

```bash
sudo ./gymbag init -d app.example.com -u :3000 -o /etc/gymbag/gymbag.json --service auto --system
sudo ./gymbag service start --system
```

::: tip `--watch` 및 `CAP_NET_BIND_SERVICE`
- `gymbag service install`은 기본적으로 `--watch=true`를 쓰므로 설정을 수정하면 데몬 재시작 없이 라우트, 방화벽 규칙, SNI 인증서가 다시 로드됩니다.
- 비루트 `--service-user`와 특권 포트(`< 1024`: `80`, `443`, `22` 및 `verify_port < 1024`)를 쓰는 시스템 단위 `systemd` 유닛에는 `AmbientCapabilities=CAP_NET_BIND_SERVICE`, `CapabilityBoundingSet=CAP_NET_BIND_SERVICE`, `NoNewPrivileges=true`가 들어갑니다.
- `--service-user`가 비루트일 때 OpenRC 유닛에는 `capabilities="^cap_net_bind_service"`가 추가됩니다.
:::

## 서비스 하위 명령어

```bash
./gymbag service start
./gymbag service stop
./gymbag service restart
./gymbag service status
./gymbag service uninstall
```

### 사용자 vs 시스템 범위

- `sudo` 없이 실행하면(`euid != 0`) `systemd`, `launchd`, `supervisor`에서 기본값이 `--user` 범위(`systemd --user`, `~/Library/LaunchAgents`)로 동작합니다.
- `root` 권한에서는 기본값이 `--system` 범위(`/etc/systemd/system`, `/Library/LaunchDaemons`, `/etc/init.d`)입니다.
- `--user` 또는 `--system`을 직접 넘겨 선택할 수 있습니다.

## Debian / Ubuntu (`.deb` + `systemd`)

릴리스 `.deb` 패키지를 설치하면 `/usr/bin/gymbag`(`cap_net_bind_service=+ep` 포함), `/etc/gymbag/Gymbagfile`(conffile), `/lib/systemd/system/gymbag.service`(`DynamicUser=yes`, `StateDirectory=gymbag`, `GYMBAG_CERT_DIR=/var/lib/gymbag/certs`)가 배치됩니다.

```bash
sudo apt install ./gymbag_<version>_linux_amd64.deb
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

## Arch Linux (`pacman` + `systemd`)

`.pkg.tar.zst` 패키지를 설치하거나 `packaging/arch/PKGBUILD`로 빌드하면 `/usr/bin/gymbag`(`cap_net_bind_service=+ep` 포함), `/etc/gymbag/Gymbagfile`, `/usr/lib/systemd/system/gymbag.service`가 설치됩니다.

```bash
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

## NixOS 모듈 (`services.gymbag`)

NixOS에서는 flake의 `nixosModules.default`를 가져와 선언형으로 설정할 수 있습니다.

```nix
{
  inputs.gymbag.url = "github:Evil-Project/gymbag";

  outputs = { nixpkgs, gymbag, ... }: {
    nixosConfigurations.my-server = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        gymbag.nixosModules.default
        {
          services.gymbag = {
            enable = true;
            openFirewall = true;
            configText = ''
              app.example.com {
                  reverse_proxy http://127.0.0.1:3000
                  verify 3
              }
            '';
          };
        }
      ];
    };
  };
}
```

이 모듈은 `DynamicUser = true`, `StateDirectory = "gymbag"`, `CAP_NET_BIND_SERVICE`로 실행되므로 root 권한 없이도 `80`과 `443` 포트를 열 수 있습니다.

## 크로스 플랫폼 유닛 파일 렌더링 (`render`)

`render`는 어떤 OS에서도 지원하는 모든 init 시스템용 서비스 정의를 출력합니다. 예를 들어 macOS에서 배포용 Linux `systemd` 유닛을 미리 생성할 때 씁니다.

```bash
./gymbag service render --type systemd --system --service-user gymbag --config /etc/gymbag/gymbag.json
./gymbag service render --type systemd --system -o ./deploy/gymbag.service
```

## `gymbag service` 플래그

| 플래그 | 설명 |
| :--- | :--- |
| `-t`, `--type <mgr>` | `auto` (기본값), `systemd`, `launchd`, `openrc`, 또는 `supervisor` |
| `-n`, `--name <name>` | 서비스 이름 (기본 `gymbag`) |
| `--label <id>` | `launchd` 레이블 (기본 `io.gymbag.<name>`) |
| `-c`, `--config <path>` | `gymbag.json`, `Gymbagfile`, 또는 `Caddyfile` 경로 |
| `--bin <path>` | `gymbag` 바이너리 경로 (기본 현재 실행 파일) |
| `--workdir <dir>` | 작업 디렉터리 (기본 설정 파일 위치) |
| `--env-file <path>` | 선택적 `.env` 파일 (`<workdir>/.env`가 있으면 자동 감지) |
| `--log-dir <dir>` | `launchd`, `openrc`, `supervisor`용 로그 디렉터리 |
| `-o`, `--output <path>` | 대상 유닛/plist/스크립트 경로 재정의 |
| `--service-user <user>` | 시스템 수준 서비스의 실행 OS 사용자 |
| `--service-group <grp>` | 시스템 수준 서비스의 실행 OS 그룹 |
| `--user` / `--system` | 사용자 또는 시스템 범위 강제 지정 |
| `--watch` | `gymbag run`에 `--watch` 전달 (기본 `true`) |
| `--enable` | `install` 시 부팅 자동 실행 활성화 (기본 `true`) |
| `--start`, `--now` | `install` 직후 서비스 시작 |
| `--no-reload` | `systemctl`/`launchctl`/`rc-update`/`supervisorctl` 실행 없이 유닛 파일만 기록/제거 |
| `-f`, `--force` | 기존 서비스 파일 덮어쓰기 |
