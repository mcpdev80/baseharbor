import os,pathlib,subprocess,concurrent.futures,json,datetime
p=pathlib.Path('evidence/board-2026-10-24/codex-cmf14-data');root=pathlib.Path('/home/marcel/dev/baseharbor-ecosystem');env=dict(os.environ,GIT_OPTIONAL_LOCKS='0');calls={}
for label,path in [('root',root),('baseharbor',root/'baseharbor'),('q114',root/'q114-fix1060'),('wizard',root/'wt-wizardfix')]:
 for kind,args in [('status',['status','--porcelain=v2','--branch','--untracked-files=normal']),('head',['rev-parse','HEAD']),('paths',['rev-parse','--show-toplevel','--git-common-dir','--absolute-git-dir']),('log',['log','-8','--format=%H %aI %cI %an %s']),('worktrees',['worktree','list','--porcelain']),('diffstat',['diff','--stat'])]:calls[label+'-'+kind]=['git','-C',str(path),*args]
calls['root-gitlinks']=['git','-C',str(root),'ls-files','--stage','--','baseharbor','baseharbor-console','baseharbor-demo','baseharbor-node-connector','baseharbor-plan','.gitmodules']
calls['root-origin-master']=['git','-C',str(root),'log','-8','--format=%H %aI %cI %an %s','refs/remotes/origin/master']
calls['root-ahead-behind']=['git','-C',str(root),'rev-list','--left-right','--count','refs/remotes/origin/master...HEAD']
calls['root-ahead-commits']=['git','-C',str(root),'log','--format=%H %aI %cI %an %s','--name-status','refs/remotes/origin/master..HEAD']
calls['module-diff']=['git','-C',str(root/'baseharbor'),'diff','--','go.mod','go.sum']
calls['module-history']=['git','-C',str(root/'baseharbor'),'log','-8','--format=%H %aI %cI %an %s','--','go.mod','go.sum']
calls['module-blobs']=['git','-C',str(root/'baseharbor'),'ls-tree','HEAD','go.mod','go.sum']
def f(t):
 n,c=t;r=subprocess.run(c,capture_output=True,text=True,env=env);(p/(n+'.txt')).write_text(r.stdout);(p/(n+'.stderr')).write_text(r.stderr);return n,r.returncode
with concurrent.futures.ThreadPoolExecutor(max_workers=6) as e:print(list(e.map(f,calls.items())))
stats={}
for path in [root/'baseharbor/go.mod',root/'baseharbor/go.sum',root/'q114-fix1060/go.mod',root/'q114-fix1060/go.sum',root/'wt-wizardfix/go.mod',root/'wt-wizardfix/go.sum',root/'.gitmodules',root/'baseharbor/site',root/'baseharbor/.git/HEAD',root/'baseharbor/.git/index',root/'.git/HEAD',root/'.git/index']:
 try:
  s=path.lstat();stats[str(path)]=dict(mtime_utc=datetime.datetime.fromtimestamp(s.st_mtime,datetime.timezone.utc).isoformat(),ctime_utc=datetime.datetime.fromtimestamp(s.st_ctime,datetime.timezone.utc).isoformat(),size=s.st_size,inode=s.st_ino,mode=oct(s.st_mode))
 except FileNotFoundError:stats[str(path)]={'exists':False}
(p/'file-metadata-before.json').write_text(json.dumps(stats,indent=2));print(json.dumps(stats,indent=2))
