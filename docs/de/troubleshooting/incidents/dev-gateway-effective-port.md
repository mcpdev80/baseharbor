# Entwickler-Gateway effektiver Port

## Symptome

```text
curl: (7) Failed to connect ... port 443
OIDC discovery unavailable
```

## Ursache

Der effektive Entwicklungs-Gateway-Port kann vom bevorzugten Port zurückfallen. Teile des Stacks nutzten zuvor den persistenten dynamischen Port, während Hörer oder Tests noch von festen 443/8443-Werten ausgehen.

## Behebung

Der anhaltende effektive Gateway-Port ist die Quelle der Wahrheit für:

- Konfiguration des Hörers,
- veröffentlichter Port Mapping,
- kanonischen Entwicklungs-URLs,
- Zugang der OIDC-Emittenten,
- Management UI-Routen,
- Demo-Akzeptierungsprüfungen.

Die Demo muss fehlschlagen, wenn der Gateway-Zustand fehlt, anstatt einen Port zu erraten.
