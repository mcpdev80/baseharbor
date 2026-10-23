# OPS-3 evidence path discipline
1. dispatch prompts always give ABSOLUTE evidence paths.
2. ghost detector checks END-marker pattern (lane-name-insensitive), fallback: any NEW *.md in evidence/ tree since dispatch (then flag WRONG-LOCATION not ghost).
