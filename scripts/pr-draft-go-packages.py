#!/usr/bin/env python3
"""Select changed Go packages and in-repository dependents for inexpensive Draft checks."""
from pathlib import Path
import subprocess
import sys


def select(changed_files, listing, module):
    package_rows = []
    all_packages = set()
    prefix = module + "/"
    for line in listing.splitlines():
        name, owner, deps = line.split("|", 2)
        package = owner or name
        if package == module or package.startswith(prefix):
            all_packages.add(package)
            package_rows.append((package, set(deps.split())))
    changed = set()
    for file in changed_files:
        path = Path(file)
        directory = path.parent.as_posix()
        changed.add(module if directory == "." else prefix + directory)
    unknown = changed - all_packages
    if unknown:
        raise ValueError("changed Go package not discoverable: " + ", ".join(sorted(unknown)))
    affected = {package for package, deps in package_rows
                if package in changed or not deps.isdisjoint(changed)}
    return sorted({"." if package == module else "./" + package[len(prefix):]
                   for package in affected})


def main():
    paths = sys.argv[1:]
    if not paths:
        return
    module_line = next((line for line in Path("go.mod").read_text().splitlines()
                        if line.startswith("module ")), None)
    if not module_line:
        raise ValueError("module declaration missing")
    module = module_line.split()[1]
    result = subprocess.run(
        ["go", "list", "-test", "-f",
         '{{.ImportPath}}|{{.ForTest}}|{{join .Deps " "}}', "./..."],
        capture_output=True, text=True, check=True)
    packages = select(paths, result.stdout, module)
    if not packages:
        raise ValueError("no affected Go packages; refusing false green")
    print("\n".join(packages))


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f"Draft Go package selection failed: {error}", file=sys.stderr)
        sys.exit(1)
