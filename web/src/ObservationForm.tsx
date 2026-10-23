import {useRef,useState} from 'react';
import {api,APIError} from './api';
import type {Task} from './types';

function canonical(value:unknown):unknown {
 if(Array.isArray(value))return value.map(canonical);
 if(value&&typeof value==='object')return Object.fromEntries(Object.entries(value).sort(([a],[b])=>a.localeCompare(b)).map(([key,item])=>[key,canonical(item)]));
 return value;
}

// Preserve raw JSON until server validation, including duplicate/unknown keys.
export function ObservationForm({task,onCancel,onSaved}:{task:Task;onCancel:()=>void;onSaved:()=>Promise<void>}) {
 const [raw,setRaw]=useState(''),[reviewed,setReviewed]=useState<Record<string,unknown>>(),[expected,setExpected]=useState(task.revision),[latest,setLatest]=useState<Task>(),[error,setError]=useState(''),[conflict,setConflict]=useState(false),[busy,setBusy]=useState(false);
 const attempts=useRef(new Map<string,string>());
 function review(){
  setError('');
  try{
   if(new TextEncoder().encode(raw).length>1024*1024)throw new Error('Report exceeds the 1 MiB request limit.');
   const value=JSON.parse(raw);
   if(!value||typeof value!=='object'||Array.isArray(value))throw new Error('Paste the report object, not a list or request envelope.');
   if(typeof value.summary!=='string'||!value.summary.trim())throw new Error('The report needs its original summary.');
   setReviewed(value);
  }catch(e){setReviewed(undefined);setError((e as Error).message)}
 }
 async function save(){
  if(!reviewed)return;setBusy(true);setError('');
  try{
   const signature=JSON.stringify(canonical(reviewed));
   let operationID=attempts.current.get(signature);
   if(!operationID){operationID=crypto.randomUUID();attempts.current.set(signature,operationID)}
   const envelope=JSON.stringify({operation_id:operationID,actor:'local-user',expected_revision:expected});
   await api('/tasks/'+task.id+'/observations',{method:'POST',body:envelope.slice(0,-1)+',"data":{"report":'+raw+'}}'});
   attempts.current.delete(signature);
   await onSaved();
  }catch(e){setError((e as Error).message);if(e instanceof APIError&&e.code==='revision_conflict')setConflict(true)}finally{setBusy(false)}
 }
 return <section><h3>Preserve a historical report</h3><p>Paste the original report object supplied by your assistant, including its evaluated task revision, context and instruction versions, observation time, and original attempt ID for execution work. Keep artifact references and evidence unchanged.</p><p>This appends history only. It does not change task status, claims, instructions or follow-up dates, and it does not establish completion.</p>
 <label>Original report JSON<textarea rows={14} spellCheck={false} value={raw} disabled={busy} onChange={e=>{setRaw(e.target.value);setReviewed(undefined)}}/></label>
 {error&&<p role="alert" className="error">{error}</p>}
 <div className="actions"><button disabled={busy||!raw.trim()} onClick={review}>Review historical report</button><button disabled={busy} onClick={onCancel}>Cancel observation</button></div>
 {reviewed&&<div className="notice"><h4>Review before saving</h4><p className="prose">{String(reviewed.summary)}</p><p>Evaluated revision: {String(reviewed.task_revision_used??'missing')} · Context version: {String(reviewed.context_version??'missing')} · Observed: {String(reviewed.observed_at??'missing')}</p><p>Stored on “{task.title}” at expected revision {expected}. The server validates the original context and evidence.</p><button className="primary" disabled={busy} onClick={save}>Save historical observation</button></div>}
 {conflict&&<div className="notice"><p>The task changed. Your original report is preserved. Review the latest state before appending it.</p><button disabled={busy} onClick={async()=>{setBusy(true);try{setLatest(await api<Task>('/tasks/'+task.id))}catch(e){setError((e as Error).message)}finally{setBusy(false)}}}>Review latest observation target</button>{latest&&<><p>{latest.title} · {latest.status} · revision {latest.revision}</p><button disabled={busy} onClick={()=>{setExpected(latest.revision);setConflict(false)}}>Use reviewed observation revision {latest.revision}</button></>}</div>}
 </section>;
}
