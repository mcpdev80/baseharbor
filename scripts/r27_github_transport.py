"""Bounded, authenticated GitHub REST GET transport for isolated evidence checks.

Call only from trusted CI with a scoped token. Never prints the token.
This transport does not issue receipts or authorize qualification.
"""
import json
import re
import urllib.request
from r27_github_origin import OriginError

_ALLOWED = re.compile(
    r"repos/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/(?:actions/runs/[1-9][0-9]*"
    r"(?:/attempts/[1-9][0-9]*/jobs\?per_page=100&page=[1-9][0-9]*)?"
    r"|pulls/[1-9][0-9]*|git/ref/heads/(?:develop|ci-policy/r27))\Z"
)
_LIMIT = 4 * 1024 * 1024


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise OriginError("EVIDENCE_ORIGIN: GitHub API redirect denied")


def _strict_pairs(pairs):
    out = {}
    for key, value in pairs:
        if key in out:
            raise OriginError("EVIDENCE_ORIGIN: duplicate GitHub API JSON key")
        out[key] = value
    return out


def _nonfinite(value):
    raise OriginError("EVIDENCE_ORIGIN: nonfinite GitHub API JSON constant")


def github_getter(token, *, opener=None):
    """Return a restricted API getter; token comes from trusted caller secret."""
    if not isinstance(token, str) or not token.strip() or "\r" in token or "\n" in token:
        raise OriginError("EVIDENCE_TRUST: GitHub credential unavailable")
    if opener is None:
        opener = urllib.request.build_opener(_NoRedirect()).open

    def get_json(path):
        if not isinstance(path, str) or not _ALLOWED.fullmatch(path):
            raise OriginError("EVIDENCE_ORIGIN: forbidden GitHub API path")
        url = "https://api.github.com/" + path
        request = urllib.request.Request(
            url, headers={
                "Authorization": "Bearer " + token,
                "Accept": "application/vnd.github+json",
                "X-GitHub-Api-Version": "2022-11-28",
            }, method="GET")
        try:
            with opener(request, timeout=15) as response:
                if response.getcode() != 200:
                    raise OriginError("EVIDENCE_ORIGIN: GitHub API status not successful")
                payload = response.read(_LIMIT + 1)
        except OriginError:
            raise
        except Exception as exc:
            raise OriginError("EVIDENCE_ORIGIN: GitHub API transport failed") from exc
        if len(payload) > _LIMIT:
            raise OriginError("EVIDENCE_ORIGIN: GitHub API response too large")
        try:
            return json.loads(payload, object_pairs_hook=_strict_pairs, parse_constant=_nonfinite)
        except (ValueError, UnicodeError) as exc:
            raise OriginError("EVIDENCE_ORIGIN: invalid GitHub API JSON") from exc

    return get_json
