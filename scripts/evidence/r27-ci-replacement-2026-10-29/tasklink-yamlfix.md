## Runde G - t-20261022-r27-evidence-publisher

1. Recipe A: receipt_verifier.py:750-753 umdrahten auf publish_card_evidence, tasklink.py:529-533 Idempotenz Remote-Bytes-Vergleich, :497-502 Duplikat-Eliminierung, _verify_durable :1122-1124 gegen origin/main
2. Recipe B: EDITED candidate Validierung auf ECHTEN Produktionspfad, Tests gegen echten plan.py mit valid+invalid Karten
3. Recipe C: Exception-Handling Restoration fuer ReceiptError, Karten-Write erst NACH _render_check, commit-ja/push-nein hinterlaesst keinen falschen Retry-Zustand
4. receipt-suite.sh (5 Suiten) exit0: test_receipt_chain OK(6), test_receipt_postrun OK(13), test_receipt_tasklink OK(59), test_receipt_verifier OK(123), test_receipt_writer OK(24)
5. Forward-Commit auf work/r27-tasklink-yamlfix

END o241-DONE
