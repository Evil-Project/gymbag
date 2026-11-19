# 시스템 서비스 설정

Gymbag에는 내장 서비스 관리자(`gymbag service`)가 포함되어 있어 다음 환경에서 백그라운드 서비스 생성, 등록, 시작, 중지, 재시작, 상태 확인 및 제거를 수행할 수 있습니다:

| 관리자 | 대상 OS / Init | 기본 유닛 경로 |
| :--- | :--- | :--- |
| `systemd` | Linux (Ubuntu, Debian, Fedora, Arch, RHEL) | `/etc/systemd/system/<name>.service` (`--user` 시 `~/.config/systemd/user/<name>.service`) |
| `launchd` | macOS (LaunchDaemons / LaunchAgents) | `/Library/LaunchDaemons/io.gymbag.<name>.plist` (`--user` 시 `~/Library/LaunchAgents/io.gymbag.<name>.plist`) |
| `openrc` | Linux (Alpine, Gentoo, Artix) | `/etc/init.d/<name>` |
| `supervisor` | Unix / 컨테이너 (`supervisord`) | `/etc/supervisor/conf.d/<name>.conf` (`--user` 시 `~/.config/supervisor/conf.d/<name>.conf`) |

---

## 두 줄 명령어 서비스 워크플로

`Gymbagfile`을 준비한 뒤:

```bash
# 1. 시스템 서비스 등록 (systemd / launchd / openrc / supervisor 자동 감지)
sudo ./gymbag service install --config /etc/gymbag/Gymbagfile

# 2. 서비스 시작
sudo ./gymbag service start
```

설정 생성과 등록을 한 번에 처리할 수도 있습니다:

```bash
sudo ./gymbag init -d app.example.com -u :3000 -o /etc/gymbag/Gymbagfile --service auto --system
sudo ./gymbag service start --system
```

::: tip 자동 `--watch` 및 `CAP_NET_BIND_SERVICE`
- `gymbag service install`은 기본적으로 `gymbag run --config <path> --watch`를 설정하므로 `Gymbagfile` 수정 시 프로세스 재시작 없이 라우트, WAF 정책 및 규칙이 핫 리로드됩니다.
- 비루트 `--service-user`와 특권 포트(`80`/`443`)로 시스템 전체 `systemd` 유닛을 등록하면 `AmbientCapabilities=CAP_NET_BIND_SERVICE`, `CapabilityBoundingSet=CAP_NET_BIND_SERVICE`, `NoNewPrivileges=true`가 자동 추가됩니다.
- OpenRC 유닛에서도 비루트 `--service-user` 지정 시 `capabilities="^cap_net_bind_service"`가 자동 포함됩니다.
:::

---

## 서비스 수명 주기 하위 명령어

```bash
# 시작, 중지, 재시작, 상태 확인
./gymbag service start
./gymbag service stop
./gymbag service restart
./gymbag service status

# 서비스 등록 및 유닛 파일 제거
./gymbag service uninstall
```

### 사용자 vs 시스템 전체 범위

- 루트가 아닌 경우(`euid != 0`) `systemd`, `launchd`, `supervisor`는 기본적으로 `--user` 범위를 사용합니다.
- `root`(`sudo`)로 실행 시 기본적으로 `--system` 범위를 사용합니다.
- `--user` 또는 `--system` 플래그로 명시적으로 재정의할 수 있습니다.

---

## NixOS 선언적 모듈 (`services.gymbag`)

NixOS에서는 flake에서 `nixosModules.default`를 가져와 선언적으로 구성할 수 있습니다:

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

`DynamicUser = true`, `StateDirectory = "gymbag"`, `CAP_NET_BIND_SERVICE`를 통해 루트 권한 없이 안전하게 `80`/`443` 포트에 바인딩합니다.

---

## Arch Linux (`pacman` + `systemd`)

Arch Linux `.pkg.tar.zst` 패키지(또는 `packaging/arch/PKGBUILD`)는 `/usr/bin/gymbag` (`cap_net_bind_service=+ep`), `/etc/gymbag/Gymbagfile`, `/usr/lib/systemd/system/gymbag.service`를 설치합니다:

```bash
sudo gymbag init -o /etc/gymbag/Gymbagfile
sudo systemctl enable --now gymbag
```

---

## 크로스 플랫폼 유닛 파일 생성 (`render`)

어느 OS에서나 다른 OS용 서비스 파일을 미리 생성할 수 있습니다:

```bash
# 표준 출력으로 systemd 유닛 미리 보기
./gymbag service render --type systemd --system --service-user gymbag --config /etc/gymbag/Gymbagfile

# 로컬 등록 없이 지정 경로에 저장
./gymbag service render --type systemd --system -o ./deploy/gymbag.service
```

---

## `gymbag service` 플래그 레퍼런스

| 플래그 | 설명 |
| :--- | :--- |
| `-t`, `--type <mgr>` | `auto` (기본값), `systemd`, `launchd`, `openrc`, `supervisor` |
| `-n`, `--name <name>` | 서비스 이름 (기본 `gymbag`) |
| `--label <id>` | `launchd` 역방향 DNS 레이블 (기본 `io.gymbag.<name>`) |
| `-c`, `--config <path>` | `Gymbagfile`, `Caddyfile`, JSON 설정 경로 |
| `--bin <path>` | `gymbag` 바이너리 경로 (기본 현재 실행 파일) |
| `--workdir <dir>` | 작업 디렉터리 (기본 설정 파일 위치) |
| `--env-file <path>` | 선택적 `.env` 파일 (`<workdir>/.env` 존재 시 자동 감지) |
| `--log-dir <dir>` | `launchd`, `openrc`, `supervisor` 로그 디렉터리 |
| `-o`, `--output <path>` | 대상 유닛 / plist / 스크립트 경로 재정의 |
| `--service-user <user>` | 시스템 서비스 실행 사용자 |
| `--service-group <grp>` | 시스템 서비스 실행 그룹 |
| `--user` / `--system` | 사용자 또는 시스템 범위 강제 지정 |
| `--watch` | `gymbag run`에 `--watch` 전달 (기본 `true`) |
| `--enable` | `install` 시 부팅 자동 시작 활성화 (기본 `true`) |
| `--start`, `--now` | `install` 직후 서비스 시작 |
| `--no-reload` | `systemctl`/`launchctl`/`rc-update`/`supervisorctl` 호출 없이 파일만 처리 |
| `-f`, `--force` | 기존 유닛 파일 덮어쓰기 |
