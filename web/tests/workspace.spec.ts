import {test,expect,type Locator} from '@playwright/test';
import {spawn,execFileSync,type ChildProcess} from 'node:child_process';
import {mkdtempSync,rmSync,readFileSync,appendFileSync,writeFileSync,cpSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {resolve,join} from 'node:path';

async function start(command:string,args:string[],prefix:string,env=process.env){
 const process=spawn(command,args,{env,stdio:['ignore','pipe','pipe']});
 const url=await new Promise<string>((resolve,reject)=>{const timer=setTimeout(()=>{process.kill();reject(new Error('startup timeout'))},15000);let output='';const check=(data:Buffer)=>{output+=data.toString();const match=output.match(new RegExp(prefix+'(http://[^\\s]+)'));if(match){clearTimeout(timer);resolve(match[1])}};process.stdout?.on('data',check);process.stderr?.on('data',check);process.on('exit',code=>{clearTimeout(timer);reject(new Error('startup exit '+code+': '+output))})});return {process,url};
}
async function stop(child?:ChildProcess){if(!child||child.exitCode!==null)return;await new Promise<void>(resolve=>{const timer=setTimeout(()=>child.kill('SIGKILL'),5000);child.once('exit',()=>{clearTimeout(timer);resolve()});child.kill('SIGTERM')})}

test('personal and delegated work remain reviewable across browser and API',async({page,request},testInfo)=>{
 const dir=mkdtempSync(join(tmpdir(),'movement-web-'));
 const binary=process.env.MOVEMENT_BINARY||resolve('../bin/movement');
 const cli=(...args:string[])=>execFileSync(binary,['--data-dir',dir,...args],{encoding:'utf8'});
 let server:ChildProcess|undefined,vite:ChildProcess|undefined;
 try{
  cli('init','--timezone','Europe/Paris');const token=cli('auth','token').trim();
  const mode=testInfo.project.name;
  const running=await start(binary,['--data-dir',dir,'serve','--listen','127.0.0.1:0',...(mode==='vite'?['--dev-origin','http://127.0.0.1:5173']:[])],'Movement API: ');server=running.process;
  let origin=running.url;
  if(mode==='vite'){const dev=await start(process.execPath,[resolve('node_modules/vite/bin/vite.js'),'--host','127.0.0.1'],'Local:\\s*',{...process.env,MOVEMENT_API_URL:origin});vite=dev.process;origin=dev.url.replace(/\/$/,'')}
  const external:string[]=[];const errors:string[]=[];
  page.on('request',r=>{if(!r.url().startsWith(origin+'/')&&!r.url().startsWith('data:'))external.push(r.url())});page.on('pageerror',e=>errors.push(e.message));
  await page.goto(origin);await page.getByLabel('Local access token').fill(token);await page.getByRole('button',{name:'Open workspace'}).click();await expect(page.getByRole('heading',{name:'My work',exact:true})).toBeVisible();
  await page.getByRole('button',{name:'New task',exact:true}).click();await page.getByLabel('Task name',{exact:true}).fill('Fill the fortnightly');
  if(mode==='vite'){
   const stylesheet=resolve('src/style.css');const marker='\n:root{--movement-hmr-proof: ready;}\n';
   await page.evaluate(()=>{(window as unknown as Record<string,unknown>).movementPageProof='same-page'});
   appendFileSync(stylesheet,marker);
   try{await expect.poll(()=>page.evaluate(()=>getComputedStyle(document.documentElement).getPropertyValue('--movement-hmr-proof').trim())).toBe('ready');await expect(page.getByLabel('Task name',{exact:true})).toHaveValue('Fill the fortnightly');expect(await page.evaluate(()=>(window as unknown as Record<string,unknown>).movementPageProof)).toBe('same-page')}
   finally{writeFileSync(stylesheet,readFileSync(stylesheet,'utf8').replace(marker,''))}
  }
  await page.getByRole('combobox',{name:'Flow',exact:true}).selectOption('waiting_for');await page.getByLabel('Waiting for whom?').fill('Nathan Delange');await page.getByLabel('Expected outcome',{exact:true}).fill('The current fortnightly contains sections A, B, C, D');await page.getByLabel('Objective',{exact:true}).fill('Record the fortnightly update');
  await page.getByRole('button',{name:'Add criterion',exact:true}).click();await page.getByLabel('Criterion 1',{exact:true}).fill('All four sections present for this period');await page.getByLabel('Investigation instructions').fill('Read https://foo.bar/in and check sections A, B, C, D for this period.');await page.getByRole('button',{name:'Save task',exact:true}).click();await expect(page.getByRole('heading',{name:'Fill the fortnightly',exact:true})).toBeVisible();
  await page.getByRole('button',{name:'Waiting for',exact:true}).click();await expect(page.locator('.task-card')).toContainText('Nathan Delange');
  const list=await request.get(running.url+'/api/v1/tasks?flow=waiting_for',{headers:{Authorization:'Bearer '+token}});const task=(await list.json()).items[0].task;
  await page.getByRole('button',{name:'Edit task',exact:true}).click();await page.getByLabel('Task name',{exact:true}).fill('Fortnightly reviewed');
  const updated=await request.patch(running.url+'/api/v1/tasks/'+task.id,{headers:{Authorization:'Bearer '+token},data:{operation_id:'parallel-edit',actor:'test-agent',expected_revision:task.revision,data:{fields:{priority:'high'}}}});expect(updated.ok()).toBeTruthy();
  await page.getByRole('button',{name:'Save task',exact:true}).click();await expect(page.getByText('This task changed while you were editing')).toBeVisible();await expect(page.getByLabel('Task name',{exact:true})).toHaveValue('Fortnightly reviewed');await page.getByRole('button',{name:'Use latest revision for my edits'}).click();await page.getByRole('button',{name:'Save task',exact:true}).click();await expect(page.getByRole('heading',{name:'Fortnightly reviewed',exact:true})).toBeVisible();
  const current=await (await request.get(running.url+'/api/v1/tasks/'+task.id,{headers:{Authorization:'Bearer '+token}})).json();const now=new Date().toISOString();
  const checked=await request.post(running.url+'/api/v1/tasks/'+task.id+'/checks',{headers:{Authorization:'Bearer '+token},data:{operation_id:'check-done',actor:'test-agent',expected_revision:current.revision,data:{report:{context_version:current.context_version,completion_version:current.completion.version,check_version:current.completion_check.version,task_revision_used:current.revision,observed_at:now,outcome:'met',summary:'A, B, C and D are present for this period.',criterion_results:[{criterion_id:current.completion.criteria[0].id,result:'met',evidence:[{reference:'https://foo.bar/in',observed_at:now,description:'All four sections verified'}]}],evidence:[],outputs:[]}}}});expect(checked.ok(),await checked.text()).toBeTruthy();
  await page.getByRole('button',{name:'Refresh',exact:true}).click();await expect(page.getByRole('button',{name:'Confirm done',exact:true})).toBeVisible();await expect(page.getByText('All four sections verified').first()).toBeVisible();await page.getByRole('button',{name:'Confirm done',exact:true}).click();await page.getByRole('button',{name:'Confirm',exact:true}).click();await expect(page.locator('.detail>.section-title')).toContainText('Done');await expect(page.locator('.task-card')).toHaveCount(0);
  // Saved browser session survives navigation; hash routes refresh correctly.
  await page.reload();await expect(page.getByRole('heading',{name:'Fortnightly reviewed',exact:true})).toBeVisible();
  await page.setViewportSize({width:390,height:844});await expect(page.getByRole('button',{name:'Edit task',exact:true})).toBeVisible();expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();await page.screenshot({path:testInfo.outputPath('workspace-mobile.png'),fullPage:true});
  // Session mutations without CSRF fail, even on the approved Origin.
  const denied=await page.evaluate(async()=>{const r=await fetch('/api/v1/tasks',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'});return r.status});expect(denied).toBe(403);
  // Late observations remain visible even on completed tasks, with explicit paging.
  let revision=(await (await request.get(running.url+'/api/v1/tasks/'+task.id,{headers:{Authorization:'Bearer '+token}})).json()).revision;
  for(let i=0;i<12;i++){
   const response=await request.post(running.url+'/api/v1/tasks/'+task.id+'/observations',{headers:{Authorization:'Bearer '+token},data:{operation_id:'late-'+i,actor:'test-agent',expected_revision:revision,data:{report:{context_version:current.context_version,completion_version:current.completion.version,check_version:current.completion_check.version,task_revision_used:current.revision,observed_at:now,outcome:'inconclusive',summary:'Late artifact '+i,criterion_results:[],evidence:[],outputs:[]}}}});
   expect(response.ok(),await response.text()).toBeTruthy();revision=(await response.json()).revision;
  }
  await page.getByRole('button',{name:'Refresh',exact:true}).click();await expect(page.getByRole('heading',{name:'Unreconciled observations · 12',exact:true})).toBeVisible();
  await page.getByRole('button',{name:'Load more observations (10 of 12)',exact:true}).click();await expect(page.getByRole('button',{name:/Load more observations/})).toHaveCount(0);
  await page.getByLabel('Reason for acknowledging an irrelevant observation').fill('Reviewed; no further action');await page.getByRole('button',{name:/Acknowledge observation/}).first().click();await expect(page.getByRole('heading',{name:'Unreconciled observations · 11',exact:true})).toBeVisible();await expect(page.locator('.detail>.section-title')).toContainText('Done');
  // Batch selection spans pages, survives a revision conflict, and retries a lost response once.
  const beforeBatch=await(await request.get(running.url+'/api/v1/tasks/'+task.id+'/context?limit=50',{headers:{Authorization:'Bearer '+token}})).json();
  const selectedIds=[beforeBatch.unreconciled_observations[0].event_id,beforeBatch.unreconciled_observations[10].event_id].sort();
  await page.getByRole('checkbox',{name:/Select observation/}).first().check();await page.getByRole('button',{name:'Load more observations (10 of 11)',exact:true}).click();await expect(page.getByRole('checkbox',{name:/Select observation/})).toHaveCount(11);await page.getByRole('checkbox',{name:/Select observation/}).last().check();
  await page.getByLabel('Reason for acknowledging an irrelevant observation').fill('Reviewed these two; retain the others');
  const parallel=await request.patch(running.url+'/api/v1/tasks/'+task.id,{headers:{Authorization:'Bearer '+token},data:{operation_id:'batch-parallel-edit',actor:'test-agent',expected_revision:beforeBatch.task.revision,data:{fields:{priority:'low'}}}});expect(parallel.ok()).toBeTruthy();
  await page.getByRole('button',{name:'Acknowledge selected observations (2)',exact:true}).click();await expect(page.getByRole('button',{name:'Refresh observation context',exact:true})).toBeVisible();await expect(page.getByLabel('Reason for acknowledging an irrelevant observation')).toHaveValue('Reviewed these two; retain the others');
  await page.getByRole('button',{name:'Refresh observation context',exact:true}).click();await page.getByRole('button',{name:'Load more observations (10 of 11)',exact:true}).click();await expect(page.getByRole('checkbox',{name:/Select observation/})).toHaveCount(11);await expect(page.getByRole('checkbox',{name:/Select observation/}).first()).toBeChecked();await expect(page.getByRole('checkbox',{name:/Select observation/}).last()).toBeChecked();
  await page.route('**/api/v1/tasks/'+task.id+'/observations',async route=>{const response=await route.fetch();expect(response.ok()).toBeTruthy();await route.abort('failed')},{times:1});
  await page.getByRole('button',{name:'Acknowledge selected observations (2)',exact:true}).click();await expect(page.getByRole('alert')).toBeVisible();await page.getByRole('button',{name:'Acknowledge selected observations (2)',exact:true}).click();await expect(page.getByRole('heading',{name:'Unreconciled observations · 9',exact:true})).toBeVisible();
  const afterBatch=await(await request.get(running.url+'/api/v1/tasks/'+task.id+'/context?limit=50',{headers:{Authorization:'Bearer '+token}})).json();expect(afterBatch.task.status).toBe('done');expect(afterBatch.task.priority).toBe('low');expect(afterBatch.task.context_version).toBe(beforeBatch.task.context_version);expect(afterBatch.history_count).toBe(beforeBatch.history_count+2);expect(afterBatch.unreconciled_observations.map((e:{event_id:string})=>e.event_id).sort()).toEqual(beforeBatch.unreconciled_observations.map((e:{event_id:string})=>e.event_id).filter((id:string)=>!selectedIds.includes(id)).sort());
  const batchHistory=await(await request.get(running.url+'/api/v1/tasks/'+task.id+'/history',{headers:{Authorization:'Bearer '+token}})).json();expect(batchHistory.at(-1).acknowledges).toEqual(selectedIds);await page.getByText('Acknowledged observations (2)',{exact:true}).click();await expect(page.locator('.timeline>li').last().locator('details li')).toHaveCount(2);
  expect(external).toEqual([]);expect(errors).toEqual([]);
  await page.getByRole('button',{name:'Lock workspace',exact:true}).click();await expect(page.getByLabel('Local access token')).toBeVisible();
 }finally{await stop(vite);await stop(server);rmSync(dir,{recursive:true,force:true})}
});


test('MCP-created task survives server restart and unsafe source text stays inert',async({page,request},testInfo)=>{
 test.skip(testInfo.project.name!=='packaged','Packaged persistence and safe rendering test');
 const dir=mkdtempSync(join(tmpdir(),'movement-mcp-browser-'));const binary=process.env.MOVEMENT_BINARY||resolve('../bin/movement');const command=['--data-dir',dir];let server:ChildProcess|undefined;
 try{
  execFileSync(binary,[...command,'init'],{encoding:'utf8'});
  const token=execFileSync(binary,[...command,'auth','token'],{encoding:'utf8'}).trim();
  let running=await start(binary,[...command,'serve','--listen','127.0.0.1:0'],'Movement API: ');server=running.process;
  const receipt=JSON.parse(execFileSync('python3',[resolve('../tests/acceptance/interfaces.py'),binary,'--seed-mcp',dir],{encoding:'utf8'}));
  await stop(server);running=await start(binary,[...command,'serve','--listen','127.0.0.1:0'],'Movement API: ');server=running.process;
  const cli=JSON.parse(execFileSync(binary,[...command,'task','get',receipt.task_id,'--json'],{encoding:'utf8'}));
  const response=await request.get(running.url+'/api/v1/tasks/'+receipt.task_id,{headers:{Authorization:'Bearer '+token}});expect(await response.json()).toEqual(cli);
  const requests:string[]=[];page.on('request',r=>{if(!r.url().startsWith(running.url+'/'))requests.push(r.url())});
  await page.goto(running.url+'/#task/'+receipt.task_id);await expect(page.getByLabel('Local access token')).toBeVisible();
  await page.keyboard.press('Tab');await expect(page.getByLabel('Local access token')).toBeFocused();await page.keyboard.insertText(token);await page.keyboard.press('Tab');await expect(page.getByRole('button',{name:'Open workspace'})).toBeFocused();await page.keyboard.press('Enter');
  await expect(page.getByRole('heading',{name:'Write an email to Kevin',exact:true})).toBeVisible();await expect(page.getByText('Read Jira status, deadline and comments, then save a Gmail draft.',{exact:true})).toBeVisible();
  await expect(page.getByText('<img src=x onerror="window.movementInjected=true">',{exact:true})).toBeVisible();expect(await page.evaluate(()=>(window as unknown as Record<string,unknown>).movementInjected)).toBeUndefined();await expect(page.locator('img')).toHaveCount(0);await expect(page.locator('a[href^="javascript:"]')).toHaveCount(0);expect(requests).toEqual([]);
  const observed=new Date().toISOString();
  const oldReport={context_version:cli.context_version,completion_version:cli.completion.version,check_version:cli.completion_check.version,task_revision_used:cli.revision,observed_at:observed,outcome:'met',summary:'Verified before the criteria changed',criterion_results:[{criterion_id:'draft',result:'met',evidence:[{reference:'fake://draft',observed_at:observed,description:'Synthetic unsent draft'}]}],evidence:[],outputs:[]};
  const checked=await request.post(running.url+'/api/v1/tasks/'+receipt.task_id+'/checks',{headers:{Authorization:'Bearer '+token},data:{operation_id:'browser-check',actor:'test-agent',expected_revision:cli.revision,data:{report:oldReport}}});expect(checked.ok()).toBeTruthy();const checkedReceipt=await checked.json();
  const revised=await request.patch(running.url+'/api/v1/tasks/'+receipt.task_id,{headers:{Authorization:'Bearer '+token},data:{operation_id:'revise-criteria',actor:'test-agent',expected_revision:checkedReceipt.revision,data:{fields:{completion:{version:1,criteria:[{id:'draft',description:'Draft includes the newly revised deadline'}]}}}}});expect(revised.ok()).toBeTruthy();const revisedReceipt=await revised.json();
  const stale=await request.post(running.url+'/api/v1/tasks/'+receipt.task_id+'/checks',{headers:{Authorization:'Bearer '+token},data:{operation_id:'stale-check',actor:'test-agent',expected_revision:revisedReceipt.revision,data:{report:oldReport}}});expect(stale.status()).toBe(409);expect((await stale.json()).error.code).toBe('context_conflict');
  const retry=await request.post(running.url+'/api/v1/tasks/'+receipt.task_id+'/checks',{headers:{Authorization:'Bearer '+token},data:{operation_id:'browser-check',actor:'test-agent',expected_revision:revisedReceipt.revision,data:{report:oldReport}}});expect(await retry.json()).toEqual(checkedReceipt);
  await page.reload();await expect(page.getByRole('heading',{name:'Write an email to Kevin',exact:true})).toBeVisible();await expect(page.getByText('Outdated context',{exact:true}).first()).toBeVisible();await expect(page.locator('.detail>.section-title')).toContainText('Ready');

 }finally{await stop(server);rmSync(dir,{recursive:true,force:true})}
});

 test('execution claims preserve shared coordination and keep tokens ephemeral',async({page,request},testInfo)=>{
 const dir=mkdtempSync(join(tmpdir(),'movement-claim-ui-'));
 const binary=process.env.MOVEMENT_BINARY||resolve('../bin/movement');
 const cli=(...args:string[])=>execFileSync(binary,['--data-dir',dir,...args],{encoding:'utf8'});
 let server:ChildProcess|undefined,vite:ChildProcess|undefined;
 try{
  cli('init','--timezone','Europe/Paris');const token=cli('auth','token').trim();
  const seed=JSON.parse(execFileSync('python3',[resolve('../tests/acceptance/interfaces.py'),binary,'--seed-mcp',dir],{encoding:'utf8'}));
  const running=await start(binary,['--data-dir',dir,'serve','--listen','127.0.0.1:0',...(testInfo.project.name==='vite'?['--dev-origin','http://127.0.0.1:5173']:[])],'Movement API: ');server=running.process;
  let origin=running.url;
  if(testInfo.project.name==='vite'){const dev=await start(process.execPath,[resolve('node_modules/vite/bin/vite.js'),'--host','127.0.0.1'],'Local:\\s*',{...process.env,MOVEMENT_API_URL:origin});vite=dev.process;origin=dev.url.replace(/\/$/,'')}
  const headers={Authorization:'Bearer '+token};
  const read=async()=>await(await request.get(running.url+'/api/v1/tasks/'+seed.task_id,{headers})).json();
  await page.goto(origin+'/#task/'+seed.task_id);await page.getByLabel('Local access token').fill(token);await page.getByRole('button',{name:'Open workspace'}).click();
  await page.getByLabel('Lease duration (seconds)').fill('120');await page.getByRole('button',{name:'Claim execution',exact:true}).click();
  await expect(page.getByRole('button',{name:'Renew claim',exact:true})).toBeEnabled();
  const owned=await read();expect(owned.status).toBe('in_progress');expect(owned.claim.actor).toBe('local-user');
  const claimToken=await page.getByLabel('Claim token',{exact:true}).inputValue();expect(claimToken.length).toBeGreaterThan(20);
  await page.getByRole('button',{name:'Edit task',exact:true}).click();await expect(page.getByLabel('Claim token for edits',{exact:true})).toHaveValue(claimToken);await page.getByRole('combobox',{name:'Priority',exact:true}).selectOption('high');await page.getByRole('button',{name:'Save task',exact:true}).click();await expect(page.getByRole('heading',{name:'Write an email to Kevin',exact:true})).toBeVisible();expect((await read()).claim).toEqual(owned.claim);expect((await read()).priority).toBe('high');await expect(page.getByLabel('Claim token',{exact:true})).toHaveValue(claimToken);
  await page.getByRole('button',{name:'Edit task',exact:true}).click();await page.getByRole('combobox',{name:'Flow',exact:true}).selectOption('waiting_for');await page.getByLabel('Waiting for whom?').fill('Nathan');await page.getByLabel('Expected outcome',{exact:true}).fill('Review the draft');await page.getByRole('button',{name:'Save task',exact:true}).click();await expect(page.getByRole('alert')).toContainText('release or explicitly override');expect((await read()).flow).toBe('actionable');expect((await read()).claim).toEqual(owned.claim);await page.getByRole('button',{name:'Cancel',exact:true}).click();
  await page.getByRole('button',{name:'Renew claim',exact:true}).click();await expect.poll(async()=>(await read()).revision).toBe(owned.revision+2);
  await expect(page.getByLabel('Claim token',{exact:true})).toHaveValue(claimToken);
  await page.reload();await expect(page.getByLabel('Claim token',{exact:true})).toHaveValue('');await expect(page.getByRole('button',{name:'Release claim',exact:true})).toBeDisabled();
  await page.getByLabel('Claim token',{exact:true}).fill('wrong-token');await page.getByRole('button',{name:'Release claim',exact:true}).click();await expect(page.getByRole('alert')).toContainText('token differs');expect((await read()).claim.attempt_id).toBe(owned.claim.attempt_id);
  await page.getByLabel('Claim token',{exact:true}).fill(claimToken);await page.getByRole('button',{name:'Release claim',exact:true}).click();await expect(page.getByRole('button',{name:'Claim execution',exact:true})).toBeVisible();expect((await read()).status).toBe('ready');
  // Another client wins while the browser still displays the previous revision.
  const current=await read();const external=await request.post(running.url+'/api/v1/tasks/'+seed.task_id+'/claim',{headers,data:{operation_id:'other-claim',actor:'another-agent',expected_revision:current.revision,data:{seconds:900}}});expect(external.ok()).toBeTruthy();
  await page.getByRole('button',{name:'Claim execution',exact:true}).click();await expect(page.getByRole('button',{name:'Refresh claim before retrying'})).toBeVisible();expect((await read()).claim.actor).toBe('another-agent');
  await page.getByRole('button',{name:'Refresh claim before retrying'}).click();await page.getByLabel('Replace the current claim with a new attempt').check();await page.getByRole('button',{name:'Replace execution claim',exact:true}).click();await expect(page.getByRole('button',{name:'Renew claim',exact:true})).toBeEnabled();expect((await read()).claim.actor).toBe('local-user');
  const replacementToken=await page.getByLabel('Claim token',{exact:true}).inputValue();const replacementTask=await read();
  await page.getByRole('button',{name:'Edit task',exact:true}).click();await page.getByLabel('Claim token for edits',{exact:true}).fill(claimToken);await page.getByRole('textbox',{name:'Notes',exact:true}).fill('This stale edit must not commit');await page.getByRole('button',{name:'Save task',exact:true}).click();await expect(page.getByRole('alert')).toContainText('token differs');expect(await read()).toEqual(replacementTask);await page.getByRole('button',{name:'Cancel',exact:true}).click();
  await page.getByRole('button',{name:'Lock workspace',exact:true}).click();await page.getByLabel('Local access token').fill(token);await page.getByRole('button',{name:'Open workspace'}).click();await page.locator('.task-card').filter({hasText:'Write an email to Kevin'}).click();await expect(page.getByLabel('Claim token',{exact:true})).toHaveValue('');
  await page.getByLabel('Claim token',{exact:true}).fill(replacementToken);await page.getByRole('button',{name:'Mark done manually',exact:true}).click();await page.getByRole('button',{name:'Confirm',exact:true}).click();await expect(page.locator('.detail>.section-title')).toContainText('Done');const completed=await read();expect(completed.claim).toBeUndefined();expect(completed.status).toBe('done');const events=JSON.parse(cli('task','history',seed.task_id,'--json'));expect(events.at(-1).changes.claim_override).toBeUndefined();
 }finally{await stop(vite);await stop(server);rmSync(dir,{recursive:true,force:true})}
});

test('structured browser reports retain evidence and reject stale context',async({page,request},testInfo)=>{
 const dir=mkdtempSync(join(tmpdir(),'movement-report-ui-'));
 const binary=process.env.MOVEMENT_BINARY||resolve('../bin/movement');
 const cli=(...args:string[])=>execFileSync(binary,['--data-dir',dir,...args],{encoding:'utf8'});
 let server:ChildProcess|undefined,vite:ChildProcess|undefined;
 try{
  cli('init','--timezone','Europe/Paris');const token=cli('auth','token').trim();
  const seed=JSON.parse(execFileSync('python3',[resolve('../tests/acceptance/interfaces.py'),binary,'--seed-mcp',dir],{encoding:'utf8'}));
  const running=await start(binary,['--data-dir',dir,'serve','--listen','127.0.0.1:0',...(testInfo.project.name==='vite'?['--dev-origin','http://127.0.0.1:5173']:[])],'Movement API: ');server=running.process;
  let origin=running.url;
  if(testInfo.project.name==='vite'){const dev=await start(process.execPath,[resolve('node_modules/vite/bin/vite.js'),'--host','127.0.0.1'],'Local:\\s*',{...process.env,MOVEMENT_API_URL:origin});vite=dev.process;origin=dev.url.replace(/\/$/,'')}
  const headers={Authorization:'Bearer '+token},url=running.url+'/api/v1/tasks/'+seed.task_id;
  const read=async()=>await(await request.get(url,{headers})).json();
  await page.goto(origin+'/#task/'+seed.task_id);await page.getByLabel('Local access token').fill(token);await page.getByRole('button',{name:'Open workspace'}).click();
  await page.getByRole('button',{name:'Claim execution',exact:true}).click();await expect(page.getByRole('button',{name:'Renew claim',exact:true})).toBeEnabled();
  await page.getByRole('button',{name:'Record work report'}).click();await page.getByLabel('Report summary').fill('Progress receipt response was lost');
  // The server commits, but the browser never receives the response. Retry must reuse its operation ID.
  await page.route('**/api/v1/tasks/'+seed.task_id+'/reports',async route=>{const response=await route.fetch();expect(response.ok()).toBeTruthy();await route.abort('failed')},{times:1});
  await page.getByRole('button',{name:'Save work report'}).click();await expect(page.getByRole('alert')).toBeVisible();await expect(page.getByLabel('Report summary')).toHaveValue('Progress receipt response was lost');
  await page.getByRole('button',{name:'Save work report'}).click();await expect(page.getByRole('button',{name:'Record work report'})).toBeVisible();
  await page.getByRole('button',{name:'Record work report'}).click();await page.getByLabel('Report summary').fill('Cannot access Jira');await page.getByLabel('Work outcome').selectOption('blocked');await page.getByRole('button',{name:'Save work report'}).click();await expect(page.locator('.detail>.section-title')).toContainText('Blocked');expect((await read()).claim).toBeFalsy();
  await page.getByRole('button',{name:'Resume',exact:true}).click();await page.getByRole('button',{name:'Confirm',exact:true}).click();await page.getByRole('button',{name:'Claim execution',exact:true}).click();await expect(page.getByRole('button',{name:'Renew claim',exact:true})).toBeEnabled();
  await page.getByRole('button',{name:'Record work report'}).click();await page.getByLabel('Report summary').fill('Saved the draft without sending');await page.getByLabel('Work outcome').selectOption('completion_proposed');await page.getByRole('button',{name:'Refresh',exact:true}).click();await expect(page.getByLabel('Report summary')).toHaveValue('Saved the draft without sending');
  await page.getByRole('button',{name:'Save work report'}).click();await expect(page.getByRole('alert')).toContainText('completion requires all criteria met');await expect(page.getByLabel('Report summary')).toHaveValue('Saved the draft without sending');
  await page.getByLabel('Criterion 1 result').selectOption('met');await page.getByRole('button',{name:'Add criterion 1 evidence',exact:true}).click();await page.getByLabel('Criterion 1 evidence 1 reference').fill('fake://gmail/draft-1');await page.getByLabel('Criterion 1 evidence 1 description').fill('Verified saved unsent draft');
  await page.getByRole('button',{name:'Add artifact',exact:true}).click();await page.getByLabel('Artifact 1 kind').fill('email_draft');await page.getByLabel('Artifact 1 provider').fill('gmail');await page.getByLabel('Artifact 1 reference').fill('fake://gmail/draft-1');await page.getByLabel('Artifact 1 description').fill('Draft for Kevin; not sent');
  await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();await page.getByLabel('Report summary').focus();await page.keyboard.press('Tab');await expect(page.getByLabel('Report observed at')).toBeFocused();await page.screenshot({path:testInfo.outputPath('report-mobile.png'),fullPage:true});
  await page.getByRole('button',{name:'Save work report'}).click();await expect(page.getByRole('button',{name:'Confirm done',exact:true})).toBeVisible();await expect(page.getByText('Draft for Kevin; not sent',{exact:true})).toBeVisible();
  // Prepare a check, then change its instructions through another adapter.
  await page.getByRole('button',{name:'Record completion check'}).click();await page.getByLabel('Report summary').fill('Retain investigation even when criteria change');await page.getByLabel('Criterion 1 result').selectOption('not_met');
  const old=await read();const edit=await request.patch(url,{headers,data:{operation_id:'report-context-edit',actor:'another-agent',expected_revision:old.revision,data:{fields:{completion:{version:1,criteria:[{id:'draft',description:'Draft must include the revised deadline'}]}}}}});expect(edit.ok()).toBeTruthy();
  await page.getByRole('button',{name:'Save completion check'}).click();await expect(page.getByRole('alert')).toBeVisible();await expect(page.getByLabel('Report summary')).toHaveValue('Retain investigation even when criteria change');
  await page.getByRole('button',{name:'Review latest report context'}).click();await expect(page.getByText('Draft must include the revised deadline',{exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'Save completion check'})).toBeDisabled();
  await page.getByRole('button',{name:/Use reviewed revision/}).click();await page.getByRole('button',{name:'Preserve as late observation'}).click();await expect(page.getByRole('heading',{name:'Unreconciled observations · 1',exact:true})).toBeVisible();expect((await read()).status).toBe('ready');
  // Reassessment uses the new context, records inconclusive and clears follow-up atomically.
  await page.getByRole('button',{name:'Record completion check'}).click();await page.getByLabel('Report summary').fill('Reassessed revised instructions; source inaccessible');await page.getByRole('combobox',{name:'Next follow-up',exact:true}).selectOption('clear');await page.getByRole('checkbox',{name:/Reconcile observation/}).check();await page.getByRole('button',{name:'Save completion check'}).click();
  await expect(page.getByRole('heading',{name:/Unreconciled observations/})).toHaveCount(0);expect((await read()).status).toBe('ready');
  const events=await(await request.get(url+'/history',{headers})).json();expect(events.filter((e:{type:string})=>e.type==='work_reported')).toHaveLength(3);const checks=events.filter((e:{type:string})=>e.type==='completion_checked');expect(checks).toHaveLength(1);expect(checks[0].check.outcome).toBe('inconclusive');expect(checks[0].check.reconciles).toHaveLength(1);
 }finally{await stop(vite);await stop(server);rmSync(dir,{recursive:true,force:true})}
});

test('stopped-store restore preserves receipts and rejects copied HTTP credentials',async({page,request},testInfo)=>{
 test.skip(testInfo.project.name!=='packaged','Restore exercises the embedded binary');
 const base=mkdtempSync(join(tmpdir(),'movement-restore-ui-')),source=join(base,'source'),restored=join(base,'restored');
 const binary=process.env.MOVEMENT_BINARY||resolve('../bin/movement');
 const cli=(dir:string,...args:string[])=>execFileSync(binary,['--data-dir',dir,...args],{encoding:'utf8'});
 let server:ChildProcess|undefined;
 try{
  cli(source,'init','--timezone','Europe/Paris');const oldToken=cli(source,'auth','token').trim();
  const seed=JSON.parse(execFileSync('python3',[resolve('../tests/acceptance/interfaces.py'),binary,'--seed-mcp',source],{encoding:'utf8'}));
  const original=await start(binary,['--data-dir',source,'serve','--listen','127.0.0.1:0'],'Movement API: ');server=original.process;
  await page.goto(original.url+'/#task/'+seed.task_id);await page.getByLabel('Local access token').fill(oldToken);await page.getByRole('button',{name:'Open workspace'}).click();await expect(page.getByRole('heading',{name:'Write an email to Kevin',exact:true})).toBeVisible();
  await stop(server);server=undefined;
  const verified=JSON.parse(cli(source,'verify'));cpSync(source,restored,{recursive:true});expect(JSON.parse(cli(restored,'verify'))).toEqual(verified);
  expect(JSON.parse(cli(restored,'auth','rotate')).rotated).toBe(true);const fresh=cli(restored,'auth','token').trim();expect(fresh).not.toBe(oldToken);expect(cli(source,'auth','token').trim()).toBe(oldToken);
  expect(cli(restored,'task','get',seed.task_id,'--json')).toBe(cli(source,'task','get',seed.task_id,'--json'));
  const a=readFileSync(join(source,'receipts', (await import('node:crypto')).createHash('sha256').update(seed.operation_id).digest('hex')+'.json'),'utf8');
  const b=readFileSync(join(restored,'receipts',(await import('node:crypto')).createHash('sha256').update(seed.operation_id).digest('hex')+'.json'),'utf8');expect(a).toBe(b);
  const running=await start(binary,['--data-dir',restored,'serve','--listen','127.0.0.1:0'],'Movement API: ');server=running.process;
  const denied=await request.get(running.url+'/api/v1/tasks',{headers:{Authorization:'Bearer '+oldToken}});expect(denied.status()).toBe(401);
  const external:string[]=[];page.on('request',r=>{if(!r.url().startsWith(running.url+'/'))external.push(r.url())});
  await page.goto(running.url+'/#task/'+seed.task_id);await expect(page.getByLabel('Local access token')).toBeVisible(); // copied client cookie cannot resume the old server session
  await page.getByLabel('Local access token').fill(fresh);await page.getByRole('button',{name:'Open workspace'}).click();await expect(page.getByRole('heading',{name:'Write an email to Kevin',exact:true})).toBeVisible();await expect(page.getByText('Read Jira status, deadline and comments, then save a Gmail draft.',{exact:true})).toBeVisible();expect(external).toEqual([]);
 }finally{await stop(server);rmSync(base,{recursive:true,force:true})}
});

test('planning options honor date boundaries, reasons and execution eligibility',async({page,request},testInfo)=>{
 const dir=mkdtempSync(join(tmpdir(),'movement-selection-ui-'));
 const binary=process.env.MOVEMENT_BINARY||resolve('../bin/movement');
 const cli=(...args:string[])=>execFileSync(binary,['--data-dir',dir,...args],{encoding:'utf8'});
 let server:ChildProcess|undefined,vite:ChildProcess|undefined;
 try{
  cli('init','--timezone','Europe/Paris');const token=cli('auth','token').trim();
  const running=await start(binary,['--data-dir',dir,'serve','--listen','127.0.0.1:0',...(testInfo.project.name==='vite'?['--dev-origin','http://127.0.0.1:5173']:[])],'Movement API: ');server=running.process;
  let origin=running.url;
  if(testInfo.project.name==='vite'){const dev=await start(process.execPath,[resolve('node_modules/vite/bin/vite.js'),'--host','127.0.0.1'],'Local:\\s*',{...process.env,MOVEMENT_API_URL:origin});vite=dev.process;origin=dev.url.replace(/\/$/,'')}
  const headers={Authorization:'Bearer '+token};let sequence=0;
  const create=async(title:string,fields:Record<string,unknown>={})=>{
   const response=await request.post(running.url+'/api/v1/tasks',{headers,data:{operation_id:'selection-'+sequence++,actor:'test',data:{title,status:'ready',context:{objective:'Inspect the fixture',constraints:[],notes:''},completion:{version:1,criteria:[{id:'verified',description:'Verified'}]},completion_check:{version:1,prompt:'Inspect fixture',criterion_ids:['verified'],source_refs:[]},...fields}}});expect(response.ok(),await response.text()).toBeTruthy();return await response.json();
  };
  const blocked=await create('Overdue blocked',{priority:'high',due_at:'2000-01-01T00:00:00Z'});
  const lease=await request.post(running.url+'/api/v1/tasks/'+blocked.task_id+'/claim',{headers,data:{operation_id:'block-claim',actor:'test',expected_revision:1,data:{seconds:900}}});const claim=await lease.json();expect(lease.ok()).toBeTruthy();
  const block=await request.post(running.url+'/api/v1/tasks/'+blocked.task_id+'/reports',{headers,data:{operation_id:'block-report',actor:'test',expected_revision:2,claim_token:claim.claim_token,data:{report:{attempt_id:(await(await request.get(running.url+'/api/v1/tasks/'+blocked.task_id,{headers})).json()).claim.attempt_id,context_version:1,completion_version:1,task_revision_used:2,observed_at:new Date().toISOString(),outcome:'blocked',summary:'Blocked fixture',criterion_results:[],evidence:[],outputs:[]}}}});expect(block.ok(),await block.text()).toBeTruthy();
  await create('Future carryover',{due_at:'2026-10-24T12:00:00Z'});
  await create('Lower boundary',{scheduled_for:'2026-10-24T22:00:00Z'});
  await create('DST final hour',{due_at:'2026-10-25T22:30:00Z'});
  await create('Upper boundary',{due_at:'2026-10-25T23:00:00Z'});
  await create('Undated fixture');
  const held=await create('Actively claimed',{due_at:'2026-10-25T12:00:00Z'});
  expect((await request.post(running.url+'/api/v1/tasks/'+held.task_id+'/claim',{headers,data:{operation_id:'active-claim',actor:'test',expected_revision:1,data:{seconds:900}}})).ok()).toBeTruthy();
  await page.goto(origin);await page.getByLabel('Local access token').fill(token);await page.getByRole('button',{name:'Open workspace'}).click();
  await page.getByText('Selection options',{exact:true}).click();await page.getByLabel('Evaluate as of (RFC 3339)').fill('2026-10-20T00:00:00Z');
  await page.getByLabel('From · Europe/Paris',{exact:true}).fill('2026-10-25');await page.getByLabel('Until · exclusive',{exact:true}).fill('2026-10-26');
  const titles=()=>page.locator('.task-card>strong');
  await expect(titles()).toHaveText(['Overdue blocked','Future carryover','Lower boundary','Actively claimed','DST final hour']);
  await expect(page.locator('.task-card').filter({hasText:'Future carryover'})).toContainText('Earlier deadline carried forward');await expect(page.locator('.task-card').filter({hasText:'Future carryover'}).locator('.overdue')).toHaveCount(0);
  await expect(page.locator('.task-card').filter({hasText:'Lower boundary'})).toContainText('Scheduled in this window');
  await page.getByLabel('Include overdue',{exact:true}).uncheck();await expect(titles()).toHaveText(['Overdue blocked','Future carryover','Lower boundary','Actively claimed','DST final hour']);await page.getByLabel('Include earlier deadline carryover').uncheck();await expect(titles()).toHaveText(['Lower boundary','Actively claimed','DST final hour']);
  await page.getByLabel('Include undated in timeframe').check();await expect(titles()).toHaveText(['Lower boundary','Actively claimed','DST final hour','Undated fixture']);
  await page.getByRole('combobox',{name:'Selection mode',exact:true}).selectOption('execution');await expect(titles()).toHaveText(['Lower boundary','DST final hour','Undated fixture']);await expect(page.locator('.task-card').first()).toContainText('Eligible at evaluation time');
  await page.getByLabel('Include overdue',{exact:true}).check();await expect(titles()).toHaveText(['Lower boundary','DST final hour','Undated fixture']);await page.getByLabel('Include overdue',{exact:true}).uncheck();
  await page.getByLabel('Evaluate as of (RFC 3339)').fill('2027-01-01T00:00:00Z');await expect(page.locator('.task-card').filter({hasText:'Actively claimed'})).toContainText('Claim expired at evaluation time; reconcile existing work');await page.getByLabel('Evaluate as of (RFC 3339)').fill('2026-10-20T00:00:00Z');await expect(titles()).toHaveText(['Lower boundary','DST final hour','Undated fixture']);
  // Pagination preserves as_of and never duplicates rows.
  await page.getByLabel('Tasks per page').fill('1');await expect(titles()).toHaveText(['Lower boundary']);await page.getByRole('button',{name:'Load more tasks'}).click();await expect(titles()).toHaveText(['Lower boundary','DST final hour']);await page.getByRole('button',{name:'Load more tasks'}).click();await expect(titles()).toHaveText(['Lower boundary','DST final hour','Undated fixture']);
  // A response for an old filter set cannot append into a new list.
  await page.getByRole('button',{name:'Refresh',exact:true}).click();await expect(titles()).toHaveText(['Lower boundary']);
  let release!:()=>void;const gate=new Promise<void>(resolve=>{release=resolve});let intercepted!:()=>void;const pendingResponse=new Promise<void>(resolve=>{intercepted=resolve});
  await page.route('**/api/v1/tasks?*',async route=>{if(new URL(route.request().url()).searchParams.has('cursor')){const response=await route.fetch();intercepted();await gate;await route.fulfill({response})}else await route.continue()});
  try{await page.getByRole('button',{name:'Load more tasks'}).click();await pendingResponse;await page.getByRole('combobox',{name:'Status',exact:true}).selectOption('blocked');await expect(titles()).toHaveCount(0)}finally{release();await page.unrouteAll({behavior:'wait'})}await expect(titles()).toHaveCount(0);
  // Exact instants preserve offsets and sub-millisecond boundaries; multiple statuses are explicit.
  await page.getByLabel('Tasks per page').fill('50');await page.getByRole('combobox',{name:'Selection mode',exact:true}).selectOption('planning');await page.getByLabel('Include undated in timeframe').uncheck();await page.getByRole('combobox',{name:'Status',exact:true}).selectOption('custom');await expect(page.getByRole('alert')).toContainText('Select at least one status');await expect(titles()).toHaveCount(0);
  const statuses=page.getByRole('group',{name:'Included statuses'});await statuses.getByRole('checkbox',{name:'Ready',exact:true}).check();await statuses.getByRole('checkbox',{name:'In progress',exact:true}).check();
  await page.getByRole('combobox',{name:'Timeframe format',exact:true}).selectOption('instant');await page.getByLabel('From (RFC 3339)',{exact:true}).fill('2026-10-25T14:00:00+02:00');await expect(page.getByRole('alert')).toContainText('Set both timeframe boundaries');await expect(titles()).toHaveCount(0);await page.getByLabel('Until (RFC 3339, exclusive)',{exact:true}).fill('2026-10-25T23:30:00+01:00');await expect(titles()).toHaveText(['Actively claimed']);
  await statuses.getByRole('checkbox',{name:'In progress',exact:true}).uncheck();await expect(titles()).toHaveCount(0);await page.getByLabel('Until (RFC 3339, exclusive)',{exact:true}).fill('2026-10-25T22:30:00.000001Z');await expect(titles()).toHaveText(['DST final hour']);
  const exactQuery=new URLSearchParams({flow:'actionable',view:'planning',from:'2026-10-25T14:00:00+02:00',to:'2026-10-25T22:30:00.000001Z',as_of:'2026-10-20T00:00:00Z',include_overdue:'false',include_carryover:'false',include_unscheduled:'false',statuses:'ready'});
  const exactResponse=await request.get(running.url+'/api/v1/tasks?'+exactQuery,{headers});expect(exactResponse.ok()).toBeTruthy();const exactPage=await exactResponse.json();expect(await titles().allTextContents()).toEqual(exactPage.items.map((item:{task:{title:string}})=>item.task.title));await expect(page.locator('.task-card')).toContainText('Due in this window');
  await page.getByLabel('From (RFC 3339)',{exact:true}).fill('not-a-date');await expect(page.getByRole('alert')).toBeVisible();await expect(titles()).toHaveCount(0);await page.getByRole('combobox',{name:'Timeframe format',exact:true}).selectOption('date');await expect(page.getByRole('alert')).toHaveCount(0);await expect(page.getByLabel('From · Europe/Paris',{exact:true})).toHaveValue('');
  await create('Nathan record',{flow:'waiting_for',delegation:{assignee:{display_name:'Nathan'},delegated_at:'2026-10-20T12:00:00Z',expected_outcome:'Review'}});await page.getByRole('button',{name:'All tasks',exact:true}).click();await page.getByLabel('Person',{exact:true}).fill('Nathan');await expect(titles()).toHaveText(['Nathan record']);
  await create('Nathan dated',{flow:'waiting_for',due_at:'2026-10-25T12:00:00Z',delegation:{assignee:{display_name:'Nathan'},delegated_at:'2026-10-20T12:00:00Z',expected_outcome:'Review dated work'}});
  await page.getByRole('button',{name:'Refresh',exact:true}).click();await expect(titles()).toHaveText(['Nathan dated','Nathan record']);
  await page.getByLabel('Only undated tasks',{exact:true}).check();await expect(titles()).toHaveText(['Nathan record']);
  await page.getByRole('button',{name:'Waiting for',exact:true}).click();await expect(page.getByLabel('Only undated tasks',{exact:true})).not.toBeChecked();await expect(titles()).toHaveText(['Nathan dated','Nathan record']);
  await page.getByLabel('Only undated tasks',{exact:true}).check();await expect(titles()).toHaveText(['Nathan record']);
  const undatedResponse=await request.get(running.url+'/api/v1/tasks?flow=waiting_for&undated_only=true',{headers});expect(undatedResponse.ok()).toBeTruthy();const undatedPage=await undatedResponse.json();expect(await titles().allTextContents()).toEqual(undatedPage.items.map((item:{task:{title:string}})=>item.task.title));
  await page.getByRole('button',{name:'Undated',exact:true}).click();await expect(page.getByLabel('Only undated tasks',{exact:true})).toBeChecked();await expect(titles()).toHaveText(['Undated fixture']);


 }finally{await stop(vite);await stop(server);rmSync(dir,{recursive:true,force:true})}
});

test('historical report import preserves terminal state and strict original evidence',async({page,request},testInfo)=>{
 const dir=mkdtempSync(join(tmpdir(),'movement-observe-ui-'));
 const binary=process.env.MOVEMENT_BINARY||resolve('../bin/movement');
 const cli=(...args:string[])=>execFileSync(binary,['--data-dir',dir,...args],{encoding:'utf8'});
 let server:ChildProcess|undefined,vite:ChildProcess|undefined;
 try{
  cli('init','--timezone','Europe/Paris');const token=cli('auth','token').trim();
  const seed=JSON.parse(execFileSync('python3',[resolve('../tests/acceptance/interfaces.py'),binary,'--seed-mcp',dir],{encoding:'utf8'}));
  const running=await start(binary,['--data-dir',dir,'serve','--listen','127.0.0.1:0',...(testInfo.project.name==='vite'?['--dev-origin','http://127.0.0.1:5173']:[])],'Movement API: ');server=running.process;
  let origin=running.url;
  if(testInfo.project.name==='vite'){const dev=await start(process.execPath,[resolve('node_modules/vite/bin/vite.js'),'--host','127.0.0.1'],'Local:\\s*',{...process.env,MOVEMENT_API_URL:origin});vite=dev.process;origin=dev.url.replace(/\/$/,'')}
  const headers={Authorization:'Bearer '+token},url=running.url+'/api/v1/tasks/'+seed.task_id;
  expect((await request.patch(url,{headers,data:{operation_id:'finish-before-observation',actor:'test',expected_revision:1,data:{action:'complete'}}})).ok()).toBeTruthy();
  const observed=new Date().toISOString();
  const report={context_version:1,completion_version:1,check_version:1,task_revision_used:1,observed_at:observed,outcome:'inconclusive',summary:'Late draft reference retained after completion',criterion_results:[],evidence:[],outputs:[{kind:'email_draft',provider:'gmail',reference:'fake://gmail/retained-draft',observed_at:observed,description:'Existing unsent draft; do not recreate'}]};
  await page.goto(origin+'/#task/'+seed.task_id);await page.getByLabel('Local access token').fill(token);await page.getByRole('button',{name:'Open workspace'}).click();await expect(page.locator('.detail>.section-title')).toContainText('Done');
  await page.getByRole('button',{name:'Preserve historical report',exact:true}).click();
  const raw=JSON.stringify(report,null,2);const duplicate=raw.replace('"summary":','"summary":"duplicate", "summary":');
  await page.getByLabel('Original report JSON').fill(duplicate);await page.getByRole('button',{name:'Review historical report',exact:true}).click();await page.getByRole('button',{name:'Save historical observation',exact:true}).click();await expect(page.getByRole('alert')).toContainText('duplicate');await expect(page.getByLabel('Original report JSON')).toHaveValue(duplicate);
  await page.getByLabel('Original report JSON').fill(raw);await page.getByRole('button',{name:'Review historical report',exact:true}).click();
  expect((await request.patch(url,{headers,data:{operation_id:'parallel-observation-edit',actor:'test',expected_revision:2,data:{fields:{priority:'high'}}}})).ok()).toBeTruthy();
  await page.getByRole('button',{name:'Save historical observation',exact:true}).click();await expect(page.getByRole('button',{name:'Review latest observation target'})).toBeVisible();await expect(page.getByLabel('Original report JSON')).toHaveValue(raw);await page.getByRole('button',{name:'Review latest observation target'}).click();await page.getByRole('button',{name:'Use reviewed observation revision 3'}).click();
  await page.route('**/api/v1/tasks/'+seed.task_id+'/observations',async route=>{const response=await route.fetch();expect(response.ok()).toBeTruthy();await route.abort('failed')},{times:1});
  await page.getByRole('button',{name:'Save historical observation',exact:true}).click();await expect(page.getByRole('alert')).toBeVisible();await expect(page.getByLabel('Original report JSON')).toHaveValue(raw);await page.getByLabel('Original report JSON').fill(JSON.stringify({...report,task_revision_used:999}));await page.getByRole('button',{name:'Review historical report',exact:true}).click();await page.getByRole('button',{name:'Save historical observation',exact:true}).click();await expect(page.getByRole('alert')).toBeVisible();await page.getByLabel('Original report JSON').fill(JSON.stringify(Object.fromEntries(Object.entries(report).reverse())));await page.getByRole('button',{name:'Review historical report',exact:true}).click();await page.getByRole('button',{name:'Save historical observation',exact:true}).click();
  await expect(page.getByRole('heading',{name:'Unreconciled observations · 1',exact:true})).toBeVisible();await expect(page.locator('.detail>.section-title')).toContainText('Done');
  const task=await(await request.get(url,{headers})).json();expect(task.revision).toBe(4);expect(task.priority).toBe('high');expect(task.status).toBe('done');
  const history=await(await request.get(url+'/history',{headers})).json();expect(history).toHaveLength(4);const event=history.find((e:{type:string})=>e.type==='observation_recorded');expect(event.observation.task_revision_used).toBe(1);expect(event.observation.outputs.map((o:{observed_at:string})=>({...o,observed_at:new Date(o.observed_at).toISOString()}))).toEqual(report.outputs);expect(event.observation.summary).toBe(report.summary);
 }finally{await stop(vite);await stop(server);rmSync(dir,{recursive:true,force:true})}
});

test('unrelated edits preserve partial checks and retained delegation',async({page,request},testInfo)=>{
 const dir=mkdtempSync(join(tmpdir(),'movement-edit-preservation-'));const binary=process.env.MOVEMENT_BINARY||resolve('../bin/movement');
 const cli=(...args:string[])=>execFileSync(binary,['--data-dir',dir,...args],{encoding:'utf8'});
 let server:ChildProcess|undefined,vite:ChildProcess|undefined;
 try{
  cli('init');const token=cli('auth','token').trim();
  const running=await start(binary,['--data-dir',dir,'serve','--listen','127.0.0.1:0',...(testInfo.project.name==='vite'?['--dev-origin','http://127.0.0.1:5173']:[])],'Movement API: ');server=running.process;
  let origin=running.url;
  if(testInfo.project.name==='vite'){const dev=await start(process.execPath,[resolve('node_modules/vite/bin/vite.js'),'--host','127.0.0.1'],'Local:\\s*',{...process.env,MOVEMENT_API_URL:origin});vite=dev.process;origin=dev.url.replace(/\/$/,'')}
  const headers={Authorization:'Bearer '+token};
  const delegation={assignee:{display_name:'Nathan'},delegated_at:'2026-10-20T12:34:56.123456Z',expected_outcome:'Fill the fortnightly'};
  const completion={version:1,criteria:[{id:'a',description:'Section A present'},{id:'b',description:'Section B present'}]};
  const completion_check={version:1,prompt:'Inspect section A only',criterion_ids:['a'],source_refs:['fake://fortnightly']};
  const created=await request.post(running.url+'/api/v1/tasks',{headers,data:{operation_id:'partial-check-seed',actor:'test',data:{title:'Partial check',status:'ready',flow:'actionable',completion,completion_check,delegation}}});expect(created.ok(),await created.text()).toBeTruthy();
  const id=(await created.json()).task_id;
  const read=()=>JSON.parse(cli('task','get',id,'--json'));
  await page.goto(origin+'/#task/'+id);await page.getByLabel('Local access token').fill(token);await page.getByRole('button',{name:'Open workspace'}).click();
  await expect(page.getByRole('heading',{name:'Partial check',exact:true})).toBeVisible();
  const missing=page.getByRole('region',{name:'Missing context'});await expect(missing).toContainText('Objective: describe the intended outcome.');await expect(missing).toContainText('Execution guidance: explain how to do the work.');await expect(missing).not.toContainText('Completion criteria:');await expect(missing).not.toContainText('Completion check:');
  await page.getByRole('button',{name:'Edit task',exact:true}).click();
  await expect(page.getByLabel('Check criterion: Section A present',{exact:true})).toBeChecked();
  await expect(page.getByLabel('Check criterion: Section B present',{exact:true})).not.toBeChecked();
  await expect(page.getByLabel('Keep delegation details')).toBeChecked();
  await page.getByRole('combobox',{name:'Priority',exact:true}).selectOption('high');
  await page.getByRole('button',{name:'Save task',exact:true}).click();
  await expect(page.getByRole('heading',{name:'Partial check',exact:true})).toBeVisible();
  let current=read();expect(current.priority).toBe('high');expect(current.context_version).toBe(1);expect(current.delegation).toEqual(delegation);expect(current.completion_check).toEqual(completion_check);
  const history=JSON.parse(cli('task','history',id,'--json'));expect(history[1].changes.fields).toEqual({priority:'high'});
  const edited=page.locator('.timeline>li').nth(1);await edited.getByText('Recorded changes',{exact:true}).focus();await page.keyboard.press('Enter');await expect(edited.locator('.history-changes')).toContainText('Priority');await expect(edited.locator('.history-changes')).toContainText('high');
  // Scope changes are explicit; adding a criterion does not silently expand an existing partial check.
  await page.getByRole('button',{name:'Edit task',exact:true}).click();await page.getByRole('button',{name:'Add criterion',exact:true}).click();await page.getByLabel('Criterion 3',{exact:true}).fill('Section C present');
  await expect(page.getByLabel('Check criterion: Section C present',{exact:true})).not.toBeChecked();
  await page.getByLabel('Check criterion: Section B present',{exact:true}).focus();await page.keyboard.press('Space');
  await page.getByRole('button',{name:'Save task',exact:true}).click();await expect(page.getByRole('heading',{name:'Partial check',exact:true})).toBeVisible();
  current=read();expect(current.completion_check.criterion_ids).toEqual(['a','b']);expect(current.completion_check.version).toBe(2);expect(current.delegation).toEqual(delegation);
  // Removing a selected criterion must remove its check reference atomically.
  await page.getByRole('button',{name:'Edit task',exact:true}).click();await page.getByRole('button',{name:'Remove criterion 1',exact:true}).click();
  await page.getByRole('button',{name:'Save task',exact:true}).click();await expect(page.getByRole('heading',{name:'Partial check',exact:true})).toBeVisible();expect(read().completion_check.criterion_ids).toEqual(['b']);
  // Flow changes preserve historical delegation, until the user explicitly removes it.
  await page.getByRole('button',{name:'Edit task',exact:true}).click();await page.getByRole('combobox',{name:'Flow',exact:true}).selectOption('waiting_for');await page.getByRole('button',{name:'Save task',exact:true}).click();await expect(page.getByRole('heading',{name:'Partial check',exact:true})).toBeVisible();
  await page.getByRole('button',{name:'Edit task',exact:true}).click();await page.getByRole('combobox',{name:'Flow',exact:true}).selectOption('actionable');await page.getByRole('button',{name:'Save task',exact:true}).click();await expect(page.getByRole('heading',{name:'Partial check',exact:true})).toBeVisible();expect(read().delegation).toEqual(delegation);
  await page.getByRole('button',{name:'Edit task',exact:true}).click();await page.getByLabel('Keep delegation details').uncheck();await page.getByRole('button',{name:'Save task',exact:true}).click();await expect(page.getByRole('heading',{name:'Partial check',exact:true})).toBeVisible();expect(read().delegation).toBeUndefined();
  await page.getByRole('button',{name:'Edit task',exact:true}).click();await page.getByLabel('Objective',{exact:true}).fill('Review the recorded changes');const unsafe='<img src=x onerror=window.historyInjected=true>';await page.getByLabel('Notes',{exact:true}).fill(unsafe);await page.getByRole('button',{name:'Save task',exact:true}).click();await expect(page.getByRole('heading',{name:'Partial check',exact:true})).toBeVisible();await expect(missing).not.toContainText('Objective:');
  const last=page.locator('.timeline>li').last();await last.getByText('Recorded changes',{exact:true}).click();await expect(last.locator('.history-changes')).toContainText(unsafe);expect(await page.evaluate(()=>(window as unknown as Record<string,unknown>).historyInjected)).toBeUndefined();await expect(last.locator('img')).toHaveCount(0);
  await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();

 }finally{await stop(vite);await stop(server);rmSync(dir,{recursive:true,force:true})}
});

test('candidate capture and stable discovery preserve user decisions',async({page},testInfo)=>{
 const dir=mkdtempSync(join(tmpdir(),'movement-capture-ui-'));const binary=process.env.MOVEMENT_BINARY||resolve('../bin/movement');
 const cli=(...args:string[])=>execFileSync(binary,['--data-dir',dir,...args],{encoding:'utf8'});let server:ChildProcess|undefined,vite:ChildProcess|undefined;
 try{
  cli('init');const token=cli('auth','token').trim();const running=await start(binary,['--data-dir',dir,'serve','--listen','127.0.0.1:0',...(testInfo.project.name==='vite'?['--dev-origin','http://127.0.0.1:5173']:[])],'Movement API: ');server=running.process;let origin=running.url;
  if(testInfo.project.name==='vite'){const dev=await start(process.execPath,[resolve('node_modules/vite/bin/vite.js'),'--host','127.0.0.1'],'Local:\\s*',{...process.env,MOVEMENT_API_URL:origin});vite=dev.process;origin=dev.url.replace(/\/$/,'')}
  await page.goto(origin);await page.getByLabel('Local access token').fill(token);await page.getByRole('button',{name:'Open workspace'}).click();await expect(page.getByRole('heading',{name:'My work',exact:true})).toBeVisible();
  const source='fake://document?a=1&b=<period>';
  const capture=async(title:string,period:string)=>{
   await page.getByRole('button',{name:'New task',exact:true}).click();await page.getByLabel('Task name',{exact:true}).fill(title);await page.getByRole('combobox',{name:'Capture as',exact:true}).selectOption('candidate');await page.getByLabel('Use a stable discovery identity').check();
   for(const [label,value] of [['Discovery producer','fortnightly-routine'],['Source provider','fake'],['Source object ID or URI',source],['Stable action key','fill-fortnightly'],['Occurrence key',period]])await page.getByLabel(label,{exact:true}).fill(value);
  };
  await capture('Discovered fortnightly','2026-W43');await page.getByRole('button',{name:'Add source',exact:true}).click();await page.getByLabel('provider',{exact:true}).fill('fake');await page.getByLabel('reference',{exact:true}).fill(source);await page.getByLabel('Source captured at (RFC 3339)',{exact:true}).fill('2026-10-20T14:34:56.123456+02:00');await page.getByRole('button',{name:'Save task',exact:true}).click();await expect(page.getByRole('heading',{name:'Discovered fortnightly',exact:true})).toBeVisible();
  const id=page.url().split('/#task/')[1];let saved=JSON.parse(cli('task','get',id,'--json'));expect(saved.status).toBe('candidate');expect(saved.sources[0].captured_at).toBe('2026-10-20T12:34:56.123456Z');expect(JSON.parse(saved.ingestion.external_key)).toEqual(['fake',source,'fill-fortnightly','2026-W43']);
  await page.getByRole('button',{name:'Inbox',exact:true}).click();await expect(page.locator('.task-card')).toContainText('Discovered fortnightly');await page.getByRole('button',{name:'Accept task',exact:true}).click();await page.getByRole('button',{name:'Confirm',exact:true}).click();await expect(page.locator('.detail>.section-title')).toContainText('Ready');
  await page.getByRole('button',{name:'Edit task',exact:true}).click();await expect(page.getByRole('heading',{name:'Discovery identity',exact:true})).toBeVisible();await expect(page.getByLabel('Discovery producer',{exact:true})).toHaveCount(0);await page.getByLabel('Task name',{exact:true}).fill('My reviewed commitment');await page.getByRole('combobox',{name:'Priority',exact:true}).selectOption('high');await page.getByLabel('Source captured at (RFC 3339)',{exact:true}).fill('2026-10-21T15:00:00+02:00');await page.getByRole('button',{name:'Save task',exact:true}).click();await expect(page.getByRole('heading',{name:'My reviewed commitment',exact:true})).toBeVisible();
  await page.getByRole('button',{name:'Dismiss',exact:true}).click();await page.getByRole('button',{name:'Confirm',exact:true}).click();await expect(page.locator('.detail>.section-title')).toContainText('Dismissed');saved=JSON.parse(cli('task','get',id,'--json'));const before=cli('task','history',id,'--json');
  await capture('Rediscovered title must not overwrite','2026-W43');await page.getByRole('button',{name:'Save task',exact:true}).click();await expect(page.getByRole('heading',{name:'My reviewed commitment',exact:true})).toBeVisible();expect(JSON.parse(cli('task','get',id,'--json'))).toEqual(saved);expect(cli('task','history',id,'--json')).toBe(before);expect(saved.sources[0].captured_at).toBe('2026-10-21T13:00:00Z');
  await capture('Next fortnightly','2026-W45');await page.getByRole('button',{name:'Save task',exact:true}).click();await expect(page.getByRole('heading',{name:'Next fortnightly',exact:true})).toBeVisible();expect(page.url().split('/#task/')[1]).not.toBe(id);await expect(page.locator('.detail>.section-title')).toContainText('Inbox');
 }finally{await stop(vite);await stop(server);rmSync(dir,{recursive:true,force:true})}
});

test('keyboard-only navigation preserves task routing and supports the task lifecycle',async({page},testInfo)=>{
 const dir=mkdtempSync(join(tmpdir(),'movement-keyboard-'));const binary=process.env.MOVEMENT_BINARY||resolve('../bin/movement');
 const cli=(...args:string[])=>execFileSync(binary,['--data-dir',dir,...args],{encoding:'utf8'});let server:ChildProcess|undefined,vite:ChildProcess|undefined;
 // Real sequential keyboard navigation: no mouse clicks, locator.focus or DOM focus calls in the test.
 const tabTo=async(target:Locator)=>{await expect(target).toBeVisible();for(let i=0;i<160;i++){if(await target.evaluate(node=>node===document.activeElement))return;await page.keyboard.press('Tab')}throw new Error('Control unreachable by Tab: '+await target.textContent())};
 const activate=async(target:Locator)=>{await tabTo(target);await page.keyboard.press('Enter')};
 try{
  cli('init');const token=cli('auth','token').trim();const seed=JSON.parse(execFileSync('python3',[resolve('../tests/acceptance/interfaces.py'),binary,'--seed-mcp',dir],{encoding:'utf8'}));
  const running=await start(binary,['--data-dir',dir,'serve','--listen','127.0.0.1:0',...(testInfo.project.name==='vite'?['--dev-origin','http://127.0.0.1:5173']:[])],'Movement API: ');server=running.process;let origin=running.url;
  if(testInfo.project.name==='vite'){const dev=await start(process.execPath,[resolve('node_modules/vite/bin/vite.js'),'--host','127.0.0.1'],'Local:\\s*',{...process.env,MOVEMENT_API_URL:origin});vite=dev.process;origin=dev.url.replace(/\/$/,'')}
  await page.goto(origin+'/#task/'+seed.task_id);await tabTo(page.getByLabel('Local access token'));await page.keyboard.insertText(token);await page.keyboard.press('Enter');await expect(page.getByRole('heading',{name:'Write an email to Kevin',exact:true})).toBeVisible();
  await activate(page.getByRole('button',{name:'Claim execution',exact:true}));await expect(page.getByRole('button',{name:'Renew claim',exact:true})).toBeEnabled();const heldToken=await page.getByLabel('Claim token',{exact:true}).inputValue();
  const route=page.url();await activate(page.getByRole('link',{name:'Skip to workspace',exact:true}));await expect(page.locator('#workspace')).toBeFocused();expect(page.url()).toBe(route);await expect(page.getByRole('heading',{name:'Write an email to Kevin',exact:true})).toBeVisible();await expect(page.getByLabel('Claim token',{exact:true})).toHaveValue(heldToken);
  await activate(page.getByRole('button',{name:'Release claim',exact:true}));await expect(page.getByRole('button',{name:'Claim execution',exact:true})).toBeVisible();
  await activate(page.getByRole('button',{name:'New task',exact:true}));await tabTo(page.getByLabel('Task name',{exact:true}));await page.keyboard.insertText('Keyboard candidate');await tabTo(page.getByRole('combobox',{name:'Capture as',exact:true}));await page.keyboard.press('i');await page.keyboard.press('Tab');await expect(page.getByRole('combobox',{name:'Capture as',exact:true})).toHaveValue('candidate');await activate(page.getByRole('button',{name:'Save task',exact:true}));await expect(page.getByRole('heading',{name:'Keyboard candidate',exact:true})).toBeVisible();const id=page.url().split('/#task/')[1];
  await activate(page.getByRole('button',{name:'Accept task',exact:true}));await activate(page.getByRole('button',{name:'Confirm',exact:true}));await expect(page.locator('.detail>.section-title')).toContainText('Ready');
  await activate(page.getByRole('button',{name:'Edit task',exact:true}));await tabTo(page.getByRole('combobox',{name:'Priority',exact:true}));await page.keyboard.press('h');await page.keyboard.press('Tab');await activate(page.getByRole('button',{name:'Save task',exact:true}));await expect(page.getByRole('heading',{name:'Keyboard candidate',exact:true})).toBeVisible();expect(JSON.parse(cli('task','get',id,'--json')).priority).toBe('high');
  const latest=page.locator('.timeline>li').last();await activate(latest.getByText('Recorded changes',{exact:true}));await expect(latest.locator('.history-changes')).toContainText('Priority');
  await activate(page.getByRole('button',{name:'Mark done manually',exact:true}));await activate(page.getByRole('button',{name:'Confirm',exact:true}));await expect(page.locator('.detail>.section-title')).toContainText('Done');expect(JSON.parse(cli('task','get',id,'--json')).status).toBe('done');
  await activate(page.getByRole('button',{name:'Reopen',exact:true}));await activate(page.getByRole('button',{name:'Confirm',exact:true}));await expect(page.locator('.detail>.section-title')).toContainText('Ready');
  await page.setViewportSize({width:390,height:844});await activate(page.getByRole('link',{name:'Skip to workspace',exact:true}));await expect(page.locator('#workspace')).toBeFocused();expect(page.url()).toContain('/#task/'+id);expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();await page.screenshot({path:testInfo.outputPath('keyboard-mobile.png'),fullPage:true});
  await activate(page.getByRole('button',{name:'Lock workspace',exact:true}));await expect(page.getByLabel('Local access token')).toBeVisible();
 }finally{await stop(vite);await stop(server);rmSync(dir,{recursive:true,force:true})}
});
