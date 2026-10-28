# PLAN-PFLICHT (ab 2026-10-29 23:0xZ, Coord-Selbstkontrolle)
Jede Mission enthaelt PFLICHTSCHRITT 0: '## PLAN' Checkliste in der Evidence-Datei anlegen
(Tool-Check, Arbeitsschritte, Endcheck) und NACH jedem Schritt aktualisieren.
Coord-Ende-Kontrolle pro Lane ist ab jetzt: ls Evidence + grep '## PLAN' + Statusharken.
Fehlt die PLAN-Liste oder ist sie nicht aktuell = Lane gilt als NICHT fertig (unabhaengig
vom END-Text) -> Nachieferungsauftrag wie rv10, bei Wiederholung Rotation.
ZUSATZ (Owner-Fund 2026-10-29): ## PLAN in der Datei reicht NICHT — jede Lane muss
zusaetzlich das native todowrite-Tool fuellen (das ist, was die Codeman-GUI zeigt).
Missionen enthalten dafuer Schritt 0: 'todowrite mit allen Schritten, dann ## PLAN-Datei'.
Grund: AGENTS.md Pflicht 6; Owner-Kritik 2026-10-29: Worker ohne Todo, Coord ohne Kontrolle.
