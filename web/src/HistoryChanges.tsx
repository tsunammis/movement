const names:Record<string,string>={fields:'Updated fields',action:'Action',status:'Status',claim:'Execution claim',claim_override:'Claim replaced',claim_token_hash:'Claim token fingerprint',follow_up_at:'Follow-up time',retired_criteria:'Removed criteria',title:'Task name',description:'Description',flow:'Flow',priority:'Priority',tags:'Tags',scheduled_for:'Scheduled time',due_at:'Deadline',context:'Context',completion:'Completion criteria',execution_guidance:'Execution guidance',completion_check:'Completion check',delegation:'Delegation',sources:'Sources'};
function Value({value}:{value:unknown}){
 if(value===null)return <span>Cleared</span>;
 if(typeof value==='string')return <span className="prose">{value}</span>;
 if(typeof value==='boolean')return <span>{value?'Yes':'No'}</span>;
 if(typeof value==='number')return <span>{value}</span>;
 return <pre>{JSON.stringify(value,null,2)}</pre>;
}
export function HistoryChanges({changes}:{changes?:Record<string,unknown>}){
 if(!changes||!Object.keys(changes).length)return null;
 return <details className="history-changes"><summary>Recorded changes</summary><dl>{Object.entries(changes).map(([key,value])=><div key={key}><dt>{names[key]||key.replaceAll('_',' ')}</dt><dd>{key==='fields'&&value&&typeof value==='object'&&!Array.isArray(value)?<dl>{Object.entries(value).map(([field,entry])=><div key={field}><dt>{names[field]||field.replaceAll('_',' ')}</dt><dd><Value value={entry}/></dd></div>)}</dl>:<Value value={value}/>}</dd></div>)}</dl></details>
}
