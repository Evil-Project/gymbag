import argparse
import asyncio
import logging
from collections.abc import Sequence

from dotenv import load_dotenv

from gymbag.config import Settings
from gymbag.context import build_context
from gymbag.errors import GymbagError
from gymbag.tls.serve import serve


def _parse_args(argv: Sequence[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        prog="gymbag",
        description="HTTPS reverse proxy with a proof-of-work web application firewall.",
    )
    parser.add_argument(
        "config_file",
        nargs="?",
        help="Path to a Caddy-like config file (defaults to ./Gymbagfile or ./Caddyfile if present).",
    )
    parser.add_argument(
        "-c",
        "--config",
        dest="config_opt",
        help="Path to a Caddy-like config file.",
    )
    parser.add_argument(
        "-d",
        "--domain",
        help="Downstream domain(s), comma-separated.",
    )
    parser.add_argument(
        "-u",
        "--upstream",
        help="Upstream URL to reverse proxy to.",
    )
    return parser.parse_args(argv)


def main(argv: Sequence[str] | None = None) -> None:
    load_dotenv()
    logging.basicConfig(level=logging.INFO, format="%(levelname)s %(name)s: %(message)s")
    args = _parse_args(argv)
    config_path = args.config_opt or args.config_file
    try:
        settings = Settings.load(
            config_path,
            domain=args.domain,
            upstream=args.upstream,
        )
        asyncio.run(_run(settings))
    except GymbagError as exc:
        logging.getLogger("gymbag").error("%s", exc)
        raise SystemExit(1) from exc


async def _run(settings: Settings) -> None:
    context = await build_context(settings)
    await serve(context)


if __name__ == "__main__":
    main()
