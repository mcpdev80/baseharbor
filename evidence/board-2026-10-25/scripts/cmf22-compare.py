#!/usr/bin/env python3
# ROLLE: REVIEW — require complete runs before comparing all failures.
import json
from pathlib import Path
p=Path(__file__).resolve().parents[1]/'cmf22-data'
summary={}
for name in ['base','head']:
    rows=[json.loads(line) for line in (p/(name+'-full.jsonl')).read_text().splitlines()]
    completed=bool(rows and rows[-1]['Action'] in ['pass','fail'] and not rows[-1].get('Test'))
    if not completed:
        raise SystemExit(name+' run incomplete; cannot establish NEW=0')
    failures=sorted(x['Test'] for x in rows if x['Action']=='fail' and x.get('Test'))
    summary[name]={'package_result':rows[-1]['Action'],'failures':failures,'top_level_failures':[x for x in failures if '/' not in x], 'passed':sum(x['Action']=='pass' and bool(x.get('Test')) for x in rows), 'skipped':sum(x['Action']=='skip' and bool(x.get('Test')) for x in rows)}
summary['new']=sorted(set(summary['head']['failures'])-set(summary['base']['failures']))
summary['removed']=sorted(set(summary['base']['failures'])-set(summary['head']['failures']))
summary['identical_failure_sets']=summary['base']['failures']==summary['head']['failures']
(p/'failure-comparison.json').write_text(json.dumps(summary,indent=2)+'\n')
print(json.dumps(summary,indent=2))
