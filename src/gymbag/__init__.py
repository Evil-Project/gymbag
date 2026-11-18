"""HTTPS reverse proxy with a proof-of-work web application firewall."""

from importlib.metadata import PackageNotFoundError, version

try:
    __version__ = version("gymbag")
except PackageNotFoundError:
    __version__ = "0.1.0"
