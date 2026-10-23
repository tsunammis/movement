import {useState} from 'react';
import {APIError,mutate} from './api';
import type {Task} from './types';

type Receipt = {claim_token?:string};

export function ClaimControls({task,token,onToken,onChanged}:{task:Task;token:string;onToken:(value:string)=>void;onChanged:()=>void}) {
 const [seconds,setSeconds]=useState(900);
 const [busy,setBusy]=useState(false);
 const [error,setError]=useState('');
 const [conflict,setConflict]=useState(false);
 const [takeover,setTakeover]=useState(false);
 const active=!!task.claim&&new Date(task.claim.expires_at).getTime()>Date.now();
 const eligible=task.flow==='actionable'&&['ready','in_progress'].includes(task.status);
 if(!eligible&&!task.claim)return null;
 async function submit(kind:'claim'|'renew'|'release') {
  setBusy(true);setError('');setConflict(false);
  try {
   const result=await mutate<Receipt>('/tasks/'+task.id+'/claim'+(kind==='claim'?'':'/'+kind),'POST',kind==='release'?{}:{seconds},task.revision,kind==='claim'&&takeover,token);
   if(kind==='claim')onToken(result.claim_token||'');
   if(kind==='release')onToken('');
   setTakeover(false);onChanged();
  } catch(e) {
   setError(e instanceof Error?e.message:'Claim operation failed');
   setConflict(e instanceof APIError&&['revision_conflict','claim_conflict'].includes(e.code));
  } finally {setBusy(false)}
 }
 return <section aria-label="Execution claim">
  <h3>Execution claim</h3>
  <p className="muted">Reserve work while you or your assistant carry it out. This does not run an agent. Review existing artifacts and unresolved observations before repeating work.</p>
  {error&&<p role="alert" className="error">{error}</p>}
  {conflict&&<button onClick={onChanged}>Refresh claim before retrying</button>}
  <label>Lease duration (seconds)<input type="number" min={30} max={3600} step={1} value={seconds} onChange={e=>setSeconds(Number(e.target.value))}/></label>
  {task.claim&&<label>Claim token<input type="password" autoComplete="off" value={token} onChange={e=>onToken(e.target.value)} aria-describedby="claim-token-help"/></label>}
  {task.claim&&<p id="claim-token-help" className="muted">Kept only while this task stays open. To manage a claim from another client, enter its token. Reloading or locking the workspace clears it.</p>}
  {active&&eligible&&<label className="check"><input type="checkbox" checked={takeover} onChange={e=>setTakeover(e.target.checked)}/>Replace the current claim with a new attempt</label>}
  <div className="actions">
   {eligible&&(!active||takeover)&&<button disabled={busy||seconds<30||seconds>3600||!Number.isInteger(seconds)} onClick={()=>submit('claim')}>{takeover?'Replace execution claim':'Claim execution'}</button>}
   {active&&!takeover&&<><button disabled={busy||!token||seconds<30||seconds>3600||!Number.isInteger(seconds)} onClick={()=>submit('renew')}>Renew claim</button><button disabled={busy||!token} onClick={()=>submit('release')}>Release claim</button></>}
  </div>
 </section>;
}
