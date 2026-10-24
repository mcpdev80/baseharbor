import subprocess,pathlib,json,base64,concurrent.futures
p=pathlib.Path('evidence/board-2026-10-24/codex-cmf8-data')
paths=[('prepare','baseharbor-plan','tasks/t-20261017-release-0-4-26-prepare.md','main'),('core923','baseharbor-plan','tasks/core-923-acceptance-new-edit-journeys-consumers.md','main'),('release','baseharbor-plan','releases/v0.4.26.md','main'),('plan-agents','baseharbor-plan','AGENTS.md','main'),('audit','baseharbor','docs/internal/pre-release-documentation-audit.md','develop'),('console-candidate','baseharbor-console','integration-candidate.json','6017de2845fa3e4e408c69038a0a5bdd7e82cad9'),('connector-candidate','baseharbor-node-connector','integration-candidate.json','e832d5651752121dea9484e18422d3e6459f9a72')]
def f(a):
 n,r,path,ref=a;c=subprocess.run(['gh','api',f'repos/mcpdev80/{r}/contents/{path}?ref={ref}'],capture_output=True,text=True);(p/(n+'.api.json')).write_text(c.stdout)
 if c.returncode==0:(p/(n+'.md')).write_bytes(base64.b64decode(json.loads(c.stdout)['content']))
 return n,c.returncode
with concurrent.futures.ThreadPoolExecutor(max_workers=5) as ex:print(list(ex.map(f,paths)))
