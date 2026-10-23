
## 2026-10-20 Maintainer-away handoff
- Coordinator pane = w1-baseharbor (74e10b98), auto-compact ON 400000 (Fenster 500K, 80%-Regel; 200K war Irrtum, gilt nur fuer 262K-Modelle) (API confirmed set; verified footer behavior at next threshold).
- Running lanes: p22 1ff8b93a (#998 hosted job, head 416486f5), p25 6f9d0533 (loader verify 8e3f417c), q17 5b9178eb (#995 recovery, Qwen).
- On each idle event: verify evidence+remote head, rm lane, continue chain (re-pin -> H freeze -> record PR -> Codex r5 -> merges #1007 then #998) then core-922/923 from prep docs.
- User decisions awaited: release qualify/publish go; never tag without it.
