"""Keep German human guidance and canonical English navigation aligned."""

from html.parser import HTMLParser
from pathlib import Path
import sys
from urllib.parse import unquote, urlsplit

import yaml


def canonical_url(path):
    route = path.removesuffix('.md')
    if route in ('index', 'README'):
        route = ''
    elif route.endswith(('/index', '/README')):
        route = route.rsplit('/', 1)[0] + '/'
    else:
        route += '/'
    return 'https://mcpdev80.github.io/baseharbor/' + route


def compare_navigation(english, german):
    if len(english) != len(german):
        raise ValueError('German navigation sections differ from English')
    for source, translated in zip(english, german):
        if len(source) != 1 or len(translated) != 1:
            raise ValueError('Navigation entry is ambiguous')
        _, expected = next(iter(source.items()))
        label, actual = next(iter(translated.items()))
        if isinstance(expected, list):
            if not isinstance(actual, list):
                raise ValueError('German navigation hierarchy differs')
            compare_navigation(expected, actual)
            continue
        local = Path('docs/de', expected).is_file()
        destination = expected if local or expected.startswith('https://') else canonical_url(expected)
        if actual != destination or (not local and not label.endswith(' (EN)')):
            raise ValueError('German navigation needs the current translation or a marked English link')


def verify_built_links():
    root = Path('site').resolve()
    if not (root / 'de/index.html').is_file():
        raise ValueError('Build both languages before checking rendered links')
    checked = 0
    broken = set()

    class Links(HTMLParser):
        def handle_starttag(self, tag, attrs):
            nonlocal checked
            for name, value in attrs:
                if name not in ('href', 'src') or not value:
                    continue
                url = urlsplit(value)
                canonical = (url.scheme == 'https' and url.netloc == 'mcpdev80.github.io'
                             and url.path.startswith('/baseharbor/'))
                if (url.scheme or url.netloc) and not canonical:
                    continue
                path = unquote(url.path)
                if not path:
                    continue
                if path.startswith('/baseharbor/'):
                    target = root / path.removeprefix('/baseharbor/')
                elif path.startswith('/'):
                    continue
                else:
                    target = (page.parent / path).resolve()
                if target.is_dir():
                    target /= 'index.html'
                checked += 1
                if not target.is_relative_to(root) or not target.exists():
                    broken.add((str(page.relative_to(root)), value))

    for page in (root / 'de').rglob('*.html'):
        Links().feed(page.read_text())
    if broken:
        raise ValueError('Broken German rendered links: ' + repr(sorted(broken)[:5]))
    print('German rendered links and assets PASS (' + str(checked) + ' references)')


def main():
    english = yaml.safe_load(Path('mkdocs.yml').read_text())
    german = yaml.safe_load(Path('mkdocs.de.yml').read_text())
    compare_navigation(english['nav'], german['nav'])
    if english['theme']['features'] != german['theme']['features']:
        raise ValueError('German Pages interaction features differ from English')
    pages = [Path('docs/index.md')]
    for directory in ('tutorials', 'explanation', 'cli', 'how-to'):
        pages.extend(Path('docs', directory).glob('*.md'))
    # The generated exact command inventory remains canonical, not manually translated.
    pages = [page for page in pages if page != Path('docs/cli/command-index.md')]
    for page in pages:
        translated = Path('docs/de', page.relative_to('docs'))
        if not translated.is_file() or not translated.read_text().strip():
            raise ValueError('Missing maintained German human guide: ' + str(page))
    print('German navigation and human-guide coverage PASS (' + str(len(pages)) + ' pages)')
    if '--built-site' in sys.argv[1:]:
        verify_built_links()


if __name__ == '__main__':
    main()
