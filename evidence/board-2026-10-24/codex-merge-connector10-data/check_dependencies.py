import subprocess,pathlib,json,concurrent.futures,datetime
r=pathlib.Path('/tmp/cmxconn-review')
cmds={'demo-main':['gh','api','repos/mcpdev80/baseharbor-demo/commits/main','--jq','.sha'],'console-main':['gh','api','repos/mcpdev80/baseharbor-console/commits/main','--jq','.sha'],'connector-main':['gh','api','repos/mcpdev80/baseharbor-node-connector/commits/main','--jq','.sha'],'pr10':['gh','api','repos/mcpdev80/baseharbor-node-connector/pulls/10'],'demo120':['gh','api','repos/mcpdev80/baseharbor-demo/pulls/120'],'console9':['gh','api','repos/mcpdev80/baseharbor-console/pulls/9'],'account':['gh','api','user','--jq','.login']}
def run(k):
 p=subprocess.run(cmds[k],text=True,capture_output=True);v={'utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'command':cmds[k],'exit':p.returncode,'stdout':p.stdout,'stderr':p.stderr};(r/(k+'.json')).write_text(json.dumps(v,indent=2))
 if p.returncode:raise RuntimeError(k+': '+p.stderr)
 return k,p.stdout
with concurrent.futures.ThreadPoolExecutor(max_workers=4) as e:
 for k,v in e.map(run,cmds):
  if k in ['pr10','demo120','console9']:
   p=json.loads(v);print(k,p['head']['sha'],p['base']['sha'],p['state'],p['merged'])
  else:print(k,v.strip())
