# ROLLE: REVIEW; exact independent full runs, sequential ordinary CLI suites.
from pathlib import Path
import subprocess,os,json
p=Path(__file__).resolve().parents[1]/'cmf22-data'
env=dict(os.environ,GOCACHE='/tmp/cmf20-go-cache')
for name in ['base','head']:
 with (p/(name+'-full.jsonl')).open('w') as out,(p/(name+'-full.stderr')).open('w') as err:
  result=subprocess.run(['go','test','./cmd/baha/','-count=1','-json'],cwd=p/name,env=env,stdout=out,stderr=err)
 (p/(name+'-full.exit')).write_text(str(result.returncode)+'\n')
 print(name,result.returncode,flush=True)
