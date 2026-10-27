import sys
from pathlib import Path
import receipt_verifier as rv

def fake_api(path):
    # emulate the producer run + attempt provenance that live() requires
    return {'id':40428162731,'run_attempt':1,'repository':{'id':1360667669,'full_name':'mcpdev80/baseharbor'},'workflow_id':352609139,'path':'.github/workflows/ci.yml','event':'pull_request','head_sha':'bd342b914c6f7114840c335b58f66e4c9b1757d5','status':'completed','conclusion':'success',
            'jobs':[
              {'id':111,'name':'PR · Public Contracts','run_id':40428162731,'head_sha':'bd342b914c6f7114840c335b58f66e4c9b1757d5','status':'completed','conclusion':'success'},
              {'id':112,'name':'PR · Trusted Plan','run_id':40428162731,'head_sha':'bd342b914c6f7114840c335b58f66e4c9b1757d5','status':'completed','conclusion':'success'}
            ],
            'artifacts':[{'id':777,'expired':False,'size_in_bytes':123,'workflow_run':{'id':40428162731,'head_sha':'bd342b914c6f7114840c335b58f66e4c9b1757d5'}}]}

def fake_zip_api(path, *, binary=True):
    import json, base64
    witness = {'schema':'baseharbor.pr-gate-plan/v1','coverage_status':'resolved','selected_gates':['integration/static/public-contracts'],'head_sha':'bd342b914c6f7114840c335b58f66e4c9b1757d5','target_base_sha':'9a630665bdbaeabda952475d804fc0d23f17609f','integration_sha':'12f3c97ee9b593833aad5de223451e2e9d4f6dd3'}
    payload = json.dumps(witness).encode()
    # zip with single entry pr-plan.json
    import io, zipfile
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, 'w') as z:
        z.writestr('pr-plan.json', payload)
    return buf.getvalue()

rv.github = fake_zip_api

REV = sys.argv[1]
try:
    rv.approve(Path('.'), REV, 'evidence/receipts/public-contracts/40428162731-1.json', '12f3c97ee9b593833aad5de223451e2e9d4f6dd3', '5ecabc17797aaf21a454cb83fab20e4ddc49eb7c')
    print('APPROVE OK')
except SystemExit as e:
    print('SYSEXIT rc=', e.code)
except Exception as e:
    print('EXC:', type(e).__name__, e)
