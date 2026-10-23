// Browser is test infrastructure; the server under test runs with an empty PATH.
import {createRequire} from 'node:module';
import assert from 'node:assert/strict';
const require=createRequire(new URL('../../web/package.json',import.meta.url));
const {chromium}=require('@playwright/test');
let input='';for await(const chunk of process.stdin)input+=chunk;
const {origin,token,task_id}=JSON.parse(input);
const browser=await chromium.launch({channel:'chrome',headless:true});
try {
 const page=await browser.newPage();
 const external=[],errors=[];
 page.on('request',request=>{if(!request.url().startsWith(origin+'/'))external.push(request.url())});
 page.on('pageerror',error=>errors.push(error.message));
 await page.goto(origin+'/#task/'+task_id);
 await page.getByLabel('Local access token').fill(token);
 await page.getByRole('button',{name:'Open workspace'}).click();
 await page.getByRole('heading',{name:'Standalone packaged task',exact:true}).waitFor();
 await page.reload();
 await page.getByRole('heading',{name:'Standalone packaged task',exact:true}).waitFor();
 await page.getByRole('button',{name:'Edit task',exact:true}).click();
 await page.getByLabel('Task name',{exact:true}).fill('Edited in isolated packaged UI');
 await page.getByRole('button',{name:'Save task',exact:true}).click();
 await page.getByRole('heading',{name:'Edited in isolated packaged UI',exact:true}).waitFor();
 assert.deepEqual(external,[]);
 assert.deepEqual(errors,[]);
 console.log('PASS real browser: embedded UI login, direct navigation, refresh and edit without external requests');
} finally {await browser.close()}
