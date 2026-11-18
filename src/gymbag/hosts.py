import ipaddress


def normalize_host(host: str) -> str:
    value = host.strip().lower().rstrip(".")
    if value.startswith("[") and value.endswith("]"):
        value = value[1:-1]
    return value


def hostname_from_header(value: str) -> str:
    header = value.strip()
    if header.startswith("["):
        end = header.find("]")
        if end != -1:
            return normalize_host(header[1:end])
    if header.count(":") == 1:
        return normalize_host(header.split(":", 1)[0])
    return normalize_host(header)


def is_public_hostname(host: str) -> bool:
    name = normalize_host(host)
    if not name or name.startswith("*."):
        return False
    if name in {"localhost", "localhost.localdomain"} or name.endswith((".local", ".localhost")):
        return False
    try:
        ipaddress.ip_address(name)
    except ValueError:
        return True
    return False
