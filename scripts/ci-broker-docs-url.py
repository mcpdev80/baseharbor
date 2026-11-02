#!/usr/bin/env python3
"""Extract the isolated broker fixture's single expected HTTPS gateway URL."""
import re
import sys
from pathlib import Path
from urllib.parse import urlsplit


def extract_url(output):
    urls = re.findall(r"Swagger/OpenAPI:\s*(\S+)", output)
    if len(urls) != 1:
        raise ValueError("broker output must contain exactly one docs URL")
    parsed = urlsplit(urls[0])
    port = parsed.port
    if (parsed.scheme != "https" or parsed.hostname != "broker-demo.baha.localhost"
            or parsed.path != "/swagger/" or parsed.username is not None
            or parsed.password is not None or parsed.query or parsed.fragment
            or (port is not None and not 1 <= port <= 65535)):
        raise ValueError("broker docs URL is not the expected isolated HTTPS gateway")
    return urls[0]


if __name__ == "__main__":
    try:
        if len(sys.argv) != 2:
            raise ValueError("expected broker output file")
        print(extract_url(Path(sys.argv[1]).read_text(encoding="utf-8")))
    except (OSError, UnicodeError, ValueError) as error:
        print("broker docs URL: " + str(error), file=sys.stderr)
        sys.exit(1)
