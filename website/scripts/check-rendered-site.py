#!/usr/bin/env python3
"""Validate the generated Hugo site without browser or npm dependencies."""

from __future__ import annotations

import sys
from collections import Counter
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import unquote, urljoin, urlparse


BASE_PATH = "/openbao-operator"
PUBLIC_HOST = "dc-tec.github.io"
MOJIBAKE_MARKERS = ("Ã", "â€", "â€™", "â€œ", "â€˜", "ï¿½", "�")


class Document(HTMLParser):
    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.ids: list[str] = []
        self.hrefs: list[str] = []
        self.text: list[str] = []

    def handle_starttag(self, _tag: str, attrs: list[tuple[str, str | None]]) -> None:
        values = dict(attrs)
        if values.get("id"):
            self.ids.append(values["id"] or "")
        if values.get("href"):
            self.hrefs.append(values["href"] or "")

    def handle_data(self, data: str) -> None:
        self.text.append(data)


def route_for(root: Path, html_file: Path) -> str:
    relative = html_file.relative_to(root).as_posix()
    if relative == "index.html":
        return "/"
    if relative.endswith("/index.html"):
        return f"/{relative.removesuffix('index.html')}"
    if relative.endswith(".html"):
        return f"/{relative.removesuffix('.html')}/"
    raise ValueError(f"unexpected HTML path: {html_file}")


def load_document(path: Path, cache: dict[Path, Document]) -> Document:
    if path not in cache:
        document = Document()
        document.feed(path.read_text(encoding="utf-8"))
        cache[path] = document
    return cache[path]


def normalize_internal_path(current_route: str, href: str) -> tuple[str, str] | None:
    parsed = urlparse(href)
    if parsed.scheme in {"mailto", "tel", "javascript"}:
        return None
    if parsed.netloc and parsed.netloc != PUBLIC_HOST:
        return None

    path = parsed.path
    if parsed.netloc == PUBLIC_HOST:
        if path != BASE_PATH and not path.startswith(f"{BASE_PATH}/"):
            return None

    if path == BASE_PATH or path.startswith(f"{BASE_PATH}/"):
        path = path.removeprefix(BASE_PATH)

    if not path:
        route = current_route
    elif path.startswith("/"):
        route = path
    else:
        route = urlparse(urljoin(f"https://local{current_route}", path)).path

    if not route.endswith("/") and not Path(route).suffix:
        route += "/"

    return route, unquote(parsed.fragment)


def main() -> int:
    if len(sys.argv) != 2:
        print(f"usage: {Path(sys.argv[0]).name} PUBLIC_DIR", file=sys.stderr)
        return 2

    root = Path(sys.argv[1]).resolve()
    if not root.is_dir():
        print(f"rendered site not found: {root}", file=sys.stderr)
        return 1

    html_files = sorted(root.rglob("*.html"))
    routes = {route_for(root, path): path for path in html_files}
    documents: dict[Path, Document] = {}
    errors: list[str] = []

    required_routes = ("/", "/docs/", "/0.5.x/", "/next/")
    for route in required_routes:
        if route not in routes:
            errors.append(f"missing required route: {route}")

    for route, html_file in routes.items():
        document = load_document(html_file, documents)
        for element_id, count in Counter(document.ids).items():
            if count > 1:
                errors.append(f"duplicate id: {route}#{element_id} ({count})")

        text = "".join(document.text)
        for marker in MOJIBAKE_MARKERS:
            if marker in text:
                errors.append(f"encoding marker {marker!r}: {route}")

        for href in document.hrefs:
            try:
                target = normalize_internal_path(route, href)
            except ValueError as error:
                errors.append(f"invalid href: {route}: {href}: {error}")
                continue
            if target is None:
                continue

            target_route, fragment = target
            target_file = routes.get(target_route)
            if target_file is None:
                static_file = root / target_route.removeprefix("/")
                if not static_file.is_file():
                    errors.append(f"missing target: {route}: {href} -> {target_route}")
                continue

            if fragment:
                target_document = load_document(target_file, documents)
                if fragment not in target_document.ids:
                    errors.append(f"missing fragment: {route}: {href}")

    print(
        f"Rendered HTML: {len(html_files)}; "
        f"duplicate IDs: {sum(error.startswith('duplicate id:') for error in errors)}; "
        f"target, fragment, and encoding errors: "
        f"{sum(not error.startswith('duplicate id:') for error in errors)}"
    )
    if errors:
        print("\n".join(errors[:100]), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
