# Podman lokale Kandidaten Bildauflösung

## Symptome

```text
image pull ... baseharbor-runtime:demo-candidate
```

oder Laufzeitverhalten, das nicht mit der ausgecheckten Quelle übereinstimmt.

## Ursache

Docker akzeptiert die lokal gebaute `baseharbor-runtime:demo-candidate` tag direkt. Podman/Quadlet kann ein unqualifiziertes Bild anders auflösen.

## Behebung

Lokale BaseHarbor-Bilder werden für Podman als:

```text
Image=localhost/baseharbor-runtime:demo-candidate
Pull=never
```

Verpflichtet:

- `c2c6fbd7`
- `7589bef4`

Dies garantiert, dass ein gezielter Podman-Akzeptierungslauf das frisch gebaute lokale Kandidatenbild verwendet.
