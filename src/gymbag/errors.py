class GymbagError(Exception):
    """Startup or configuration failure an operator can act on."""


class CertificateError(GymbagError):
    """A TLS certificate could not be loaded or issued."""
