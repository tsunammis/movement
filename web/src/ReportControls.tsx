import {useState} from 'react';
import {ObservationForm} from './ObservationForm';
import {api,APIError,mutate} from './api';
import type {Task,Packet,Report,Evidence} from './types';

type Mode='work'|'check';
const timestamp=()=>new Date().toISOString();
const emptyEvidence=():Evidence=>({reference:'',description:'',observed_at:timestamp()});
function EvidenceFields({items,onChange,label}:{items:Evidence[];onChange:(value:Evidence[])=>void;label:string}) {
 return <fieldset><legend>{label}</legend>{items.map((item,i)=><div key={i} className="entry">
  {(['reference','description','observed_at','excerpt'] as const).map(field=><label key={field}>{label} {i+1} {field.replaceAll('_',' ')}<input value={item[field]||''} required={field==='reference'||field==='observed_at'} onChange={e=>onChange(items.map((v,n)=>n===i?{...v,[field]:e.target.value}:v))}/></label>)}
  <button type="button" onClick={()=>onChange(items.filter((_,n)=>n!==i))}>Remove {label.toLowerCase()} {i+1}</button>
 </div>)}<button type="button" onClick={()=>onChange([...items,emptyEvidence()])}>Add {label.toLowerCase()}</button></fieldset>;
}
export function ReportControls({packet,token,onChanged}:{packet:Packet;token:string;onChanged:()=>Promise<void>}) {
 const [mode,setMode]=useState<Mode>();
 const [observing,setObserving]=useState(false);
 const [snapshot,setSnapshot]=useState<Task>();
 const task=packet.task;
 function open(value:Mode){setSnapshot(task);setMode(value)}
 if(observing)return <ObservationForm task={task} onCancel={()=>setObserving(false)} onSaved={async()=>{await onChanged();setObserving(false)}}/>;
 if(mode&&snapshot)return <ReportForm key={mode+snapshot.revision} mode={mode} snapshot={snapshot} packet={packet} token={token} onCancel={()=>setMode(undefined)} onSaved={async()=>{await onChanged();setMode(undefined)}}/>;
 const openTask=!['candidate','done','dismissed'].includes(task.status);
 return <section><h3>Record an investigation or work</h3><p className="muted">Record actions already performed and what was observed. Movement does not access sources or perform the task.</p><div className="actions">
 {openTask&&task.claim&&task.flow==='actionable'&&<button onClick={()=>open('work')}>Record work report</button>}
 {openTask&&task.completion_check&&<button onClick={()=>open('check')}>Record completion check</button>}
 <button onClick={()=>setObserving(true)}>Preserve historical report</button>
 </div></section>;
}
function ReportForm({mode,snapshot,packet,token,onCancel,onSaved}:{mode:Mode;snapshot:Task;packet:Packet;token:string;onCancel:()=>void;onSaved:()=>Promise<void>}) {
 const [report,setReport]=useState<Report>(()=>({context_version:snapshot.context_version,completion_version:snapshot.completion.version,task_revision_used:snapshot.revision,...(mode==='work'?{attempt_id:snapshot.claim?.attempt_id,guidance_version:snapshot.execution_guidance?.version||0}:{check_version:snapshot.completion_check?.version}),observed_at:timestamp(),summary:'',outcome:mode==='work'?'progress':'inconclusive',criterion_results:snapshot.completion.criteria.map(c=>({criterion_id:c.id,result:'unknown',evidence:[]})),evidence:[],outputs:[],reconciles:[]}));
 const [expected,setExpected]=useState(snapshot.revision),[latest,setLatest]=useState<Task>(),[error,setError]=useState(''),[busy,setBusy]=useState(false),[conflict,setConflict]=useState(false);
 const [follow,setFollow]=useState('unchanged'),[followAt,setFollowAt]=useState('');
 const aggregate=report.criterion_results.some(c=>c.result==='not_met')?'not_met':report.criterion_results.length>0&&report.criterion_results.every(c=>c.result==='met')?'met':'inconclusive';
 const changedContext=latest&&latest.context_version!==snapshot.context_version;
 const changedAttempt=mode==='work'&&latest&&latest.claim?.attempt_id!==snapshot.claim?.attempt_id;
 const currentReport={...report,outcome:mode==='check'?aggregate:report.outcome};
 async function readLatest(){setBusy(true);try{const t=await api<Task>('/tasks/'+snapshot.id);setLatest(t)}catch(e){setError((e as Error).message)}finally{setBusy(false)}}
 async function submit(observation=false){
  setBusy(true);setError('');
  try {
   const data:{report:Report;next_follow_up_at?:string|null}={report:currentReport};
   if(mode==='check'&&!observation&&follow!=='unchanged')data.next_follow_up_at=follow==='clear'?null:followAt;
   await mutate('/tasks/'+snapshot.id+(observation?'/observations':mode==='check'?'/checks':'/reports'),'POST',data,expected,false,observation?undefined:token);
   await onSaved();
  }catch(e){setError((e as Error).message);if(e instanceof APIError&&['revision_conflict','context_conflict','claim_conflict','assessment_conflict','invalid_transition'].includes(e.code))setConflict(true)}finally{setBusy(false)}
 }
 return <section><h3>{mode==='check'?'Completion check report':'Work report'}</h3><p>Evaluated task revision {snapshot.revision}. Observation times use RFC 3339 with an offset, for example {timestamp()}. Enter when the source was actually observed.</p>
 <form onSubmit={e=>{e.preventDefault();void submit()}}>
 <fieldset disabled={busy}><legend>Report details</legend>
 <label>Report summary<textarea required value={report.summary} onChange={e=>setReport({...report,summary:e.target.value})}/></label>
 <label>Report observed at<input required value={report.observed_at} onChange={e=>setReport({...report,observed_at:e.target.value})}/></label>
 {mode==='work'?<label>Work outcome<select value={report.outcome} onChange={e=>setReport({...report,outcome:e.target.value})}><option value="progress">Progress</option><option value="blocked">Blocked</option><option value="completion_proposed">Propose completion</option></select></label>:<p>Computed check outcome: <strong>{aggregate.replaceAll('_',' ')}</strong></p>}
 <label>Next steps<textarea value={report.next_steps||''} onChange={e=>setReport({...report,next_steps:e.target.value})}/></label>
 {report.criterion_results.map((result,i)=><fieldset key={result.criterion_id}><legend>{snapshot.completion.criteria[i].description}</legend><label>Criterion {i+1} result<select value={result.result} onChange={e=>setReport({...report,criterion_results:report.criterion_results.map((r,n)=>n===i?{...r,result:e.target.value}:r)})}><option value="unknown">Unknown / inaccessible</option><option value="not_met">Not met</option><option value="met">Met</option></select></label><EvidenceFields label={'Criterion '+(i+1)+' evidence'} items={result.evidence} onChange={evidence=>setReport({...report,criterion_results:report.criterion_results.map((r,n)=>n===i?{...r,evidence}:r)})}/></fieldset>)}
 <EvidenceFields label="General evidence" items={report.evidence} onChange={evidence=>setReport({...report,evidence})}/>
 <fieldset><legend>Produced artifacts</legend>{report.outputs.map((output,i)=><div className="entry" key={i}>{(['kind','provider','reference','description','observed_at'] as const).map(field=><label key={field}>Artifact {i+1} {field.replaceAll('_',' ')}<input required={field!=='description'} value={output[field]} onChange={e=>setReport({...report,outputs:report.outputs.map((o,n)=>n===i?{...o,[field]:e.target.value}:o)})}/></label>)}<button type="button" onClick={()=>setReport({...report,outputs:report.outputs.filter((_,n)=>n!==i)})}>Remove artifact {i+1}</button></div>)}<button type="button" onClick={()=>setReport({...report,outputs:[...report.outputs,{kind:'',provider:'',reference:'',description:'',observed_at:timestamp()}]})}>Add artifact</button></fieldset>
 {mode==='check'&&<><label>Next follow-up<select value={follow} onChange={e=>setFollow(e.target.value)}><option value="unchanged">Leave unchanged</option><option value="clear">Clear schedule</option><option value="set">Schedule next check</option></select></label>{follow==='set'&&<label>Next follow-up at<input required value={followAt} onChange={e=>setFollowAt(e.target.value)} placeholder="RFC 3339 timestamp with offset"/></label>}</>}
 {packet.unreconciled_count>0&&<fieldset><legend>Reconcile observations</legend><p>Only select evidence you have reviewed against the current instructions. {packet.unreconciled_observations.length} of {packet.unreconciled_count} observations loaded; use the observations section to load the rest before repeating external work.</p>{packet.unreconciled_observations.map(event=><label className="check" key={event.event_id}><input type="checkbox" checked={report.reconciles?.includes(event.event_id)||false} onChange={e=>setReport({...report,reconciles:e.target.checked?[...(report.reconciles||[]),event.event_id]:report.reconciles?.filter(id=>id!==event.event_id)})}/>Reconcile observation {event.sequence}: {event.summary}</label>)}</fieldset>}
 </fieldset>
 {error&&<p role="alert" className="error">{error}</p>}
 {conflict&&<div className="notice"><p>Your report is preserved. Review the latest task before retrying. Changed instructions require a fresh investigation; a new storage revision alone does not make old evidence current.</p><button type="button" disabled={busy} onClick={readLatest}>Review latest report context</button>{latest&&<><p>Latest: {latest.title} · revision {latest.revision} · {latest.status}</p><p>{latest.context.objective}</p><ul>{latest.completion.criteria.map(c=><li key={c.id}>{c.description}</li>)}</ul>{changedContext||changedAttempt?<p>Instructions or execution attempt changed. Preserve this report as an observation, then reopen a report after reassessing the current task.</p>:<p>The evaluated instructions and attempt are unchanged.</p>}<button type="button" disabled={busy} onClick={()=>setExpected(latest.revision)}>Use reviewed revision {latest.revision}</button></>}
 <button type="button" disabled={busy||!report.summary.trim()} onClick={()=>submit(true)}>Preserve as late observation</button></div>}
 <p className="muted">Met criteria require supporting evidence. Proposed completion enters review; it does not confirm done. Artifact references record what exists, not permission to send or repeat an action.</p>
 <div className="actions"><button className="primary" disabled={busy||!!changedContext||!!changedAttempt}>{mode==='check'?'Save completion check':'Save work report'}</button><button type="button" disabled={busy} onClick={onCancel}>Cancel report</button></div>
 </form></section>;
}
