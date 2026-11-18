FROM python:3.14-slim-bookworm

COPY --from=ghcr.io/astral-sh/uv:latest /uv /uvx /bin/

RUN useradd --system --uid 10001 --home-dir /app --shell /usr/sbin/nologin gymbag

WORKDIR /app

ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    UV_NO_DEV=1 \
    UV_PYTHON_DOWNLOADS=never \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PATH="/app/.venv/bin:$PATH" \
    GYMBAG_BIND=0.0.0.0

COPY pyproject.toml uv.lock README.md LICENSE Gymbagfile ./
COPY frontend ./frontend
COPY src ./src
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --no-dev
COPY --chmod=755 docker/entrypoint.sh /usr/local/bin/gymbag-entrypoint

RUN mkdir -p /app/certs \
    && chown -R gymbag:gymbag /app

EXPOSE 80 443

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD ["python", "-c", "import socket; s=socket.create_connection(('127.0.0.1', 80), 3); s.close()"]

ENTRYPOINT ["gymbag-entrypoint"]
CMD ["python", "-m", "gymbag"]
