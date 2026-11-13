const token=location.hash.slice(1),headers={'X-Session-Token':token};let file='',busy=false;
const $=id=>document.getElementById(id);
let eventDisconnected=false, exiting=false, preferredModel='', modelTranslation={};
async function request(url,method='GET',data){const opts={method,headers:{...headers}};if(data!==undefined){opts.headers['Content-Type']='application/json';opts.body=JSON.stringify(data)}const res=await fetch(url,opts);if(!res.ok)throw Error((await res.text()).trim());return res.json()}
function log(s){const p=$('log');p.append(document.createTextNode(s+'\n'));p.scrollTop=p.scrollHeight}
function logNotice(s){const p=$('log'),el=document.createElement('strong');el.textContent=s;p.append(el,document.createTextNode('\n'));p.scrollTop=p.scrollHeight}
function controls(){$('start').disabled=busy||!file||!$('model').value;$('stop').disabled=!busy;$('pick').disabled=busy}
function syncTranslationAvailability(){
 const model=$('model').value,unsupported=modelTranslation[model]===false;
 $('translate').disabled=unsupported;
 if(unsupported)$('translate').checked=false;
 $('translation-warning').hidden=!unsupported;
}
function setTheme(theme){document.documentElement.dataset.theme=theme;$('theme').textContent=theme==='dark'?'☀ Light':'☾ Dark'}
$('theme').onclick=async()=>{const before=document.documentElement.dataset.theme||'light';const next=before==='dark'?'light':'dark';setTheme(next);try{await request('/api/preferences/theme','POST',{theme:next})}catch(e){setTheme(before);log('Unable to save theme: '+e.message)}};
async function refresh(){try{const r=await request('/api/models'),s=$('model'),old=s.value;modelTranslation=r.translation_supported||{};s.replaceChildren();for(const name of r.models.filter(name=>!name.startsWith("ggml-silero-")))s.add(new Option(name,name));if(r.models.includes(old))s.value=old;else if(r.models.includes(preferredModel))s.value=preferredModel;if(!r.models.length){s.add(new Option('Click Manage models to add models',''));if(!window.noModelsNotified){logNotice('No models found (or models directory missing). Check your configuration or click Manage models to download one.');window.noModelsNotified=true}}else{window.noModelsNotified=false}syncTranslationAvailability();controls()}catch(e){log('Models: '+e.message)}}
$('pick').onclick=async()=>{try{const r=await request('/api/select-file','POST');if(r.status==='ok'){file=r.path;$('filename').textContent=r.name;$('filepath').textContent=' ('+r.path+')';$('fileinfo').title=r.path;controls()}}catch(e){log('File selection: '+e.message)}};
$('refresh').onclick=openModels;$('open-models-folder').onclick=async()=>{try{await request('/api/models-folder','POST')}catch(e){$('model-admin').textContent='Unable to open models folder: '+e.message}};$('model').onchange=async()=>{syncTranslationAvailability();controls();const model=$('model').value;if(!model)return;try{await request('/api/preferences/model','POST',{model});preferredModel=model;}catch(e){log('Unable to save selected model: '+e.message)}};
$('language').onchange=async()=>{const language=$('language').value;try{await request('/api/preferences/language','POST',{language})}catch(e){log('Unable to save selected language: '+e.message)}};
$('start').onclick=async()=>{busy=true;controls();hideResult();$('log').textContent='';try{await request('/api/jobs','POST',{file,model:$('model').value,language:$('language').value,translate:$('translate').checked,word_timestamps:$('word-timestamps').checked,textgrid:$('textgrid').checked,cpu:$('cpu').checked,glossary_enabled:$('glossary-enabled').checked,glossary_hint:$('glossary-hint').value,carry_initial_prompt:$('carry-initial-prompt').checked,experimental_features:$('experimental-features').checked})}catch(e){busy=false;controls();log('Error: '+e.message)}};
function hideResult(){$('result-bar').hidden=true;$('result-name').textContent=''}
function showResult(path){$('result-name').textContent=path.split(/[\\/]/).pop();$('result-name').title=path;$('result-bar').hidden=false}
async function resultAction(action){
 if(action==='delete'&&!confirm('Delete this ZIP archive? The source audio/video will not be deleted.'))return;
 try{await request('/api/result','POST',{action});if(action==='delete')hideResult()}
 catch(e){log('Archive: '+e.message)}
}
$('result-show').onclick=()=>resultAction('show');
$('result-delete').onclick=()=>resultAction('delete');
$('clear-log').onclick=async()=>{$('log').textContent='';try{await request('/api/log/clear','POST')}catch(e){log('Clear log: '+e.message)}};
$('textgrid').onchange=()=>{if($('textgrid').checked)$('word-timestamps').checked=true};
$('word-timestamps').onchange=()=>{if(!$('word-timestamps').checked)$('textgrid').checked=false};
async function copyPanel(id,buttonId){const button=$(buttonId),label=button.textContent;try{await navigator.clipboard.writeText($(id).textContent);button.textContent='Copied';setTimeout(()=>{button.textContent=label},1400)}catch(e){log('Copy failed: '+e.message)}}
$('copy-log').onclick=()=>copyPanel('log','copy-log');
$('copy-diagnostics').onclick=()=>copyPanel('diagnostics','copy-diagnostics');
$('stop').onclick=async()=>{try{await request('/api/stop','POST');log('Stop requested…')}catch(e){log('Error: '+e.message)}};
async function connectEvents(){
  if(exiting)return;
  try {
    const response=await fetch('/api/events',{headers});
    if(!response.ok)throw Error('Event stream HTTP '+response.status);
    if(eventDisconnected){log('Connection to the local server restored.');eventDisconnected=false;}
    const reader=response.body.getReader(),decoder=new TextDecoder();let buf='';
    for(;;){
      const {value,done}=await reader.read();
      if(done)break;
      buf+=decoder.decode(value,{stream:true});
      let i;
      while((i=buf.indexOf('\n\n'))>=0){
        const line=buf.slice(0,i);buf=buf.slice(i+2);
        if(!line.startsWith('data: '))continue;
        const e=JSON.parse(line.slice(6));
        if(e.message)log(e.message);
        if(e.line)log(e.line);
        if(e.type==='done'){for(const p of e.outputs||[])log('Created: '+p);if(e.archive)showResult(e.archive);else hideResult();busy=false;controls()}
        else if(e.type==='archive-deleted'){hideResult()}
        else if(e.type==='error'||e.state==='stopped'){busy=false;controls()}
      }
    }
    throw Error('Event stream closed');
  }catch(e){
    if(!eventDisconnected){
      eventDisconnected=true;
      log('😴 Connection to the local server lost, possibly due to inactivity. If WhisperGUI has closed, restart it using its application icon.');
    }
  }
  if(!exiting)setTimeout(connectEvents,document.hidden?5000:1500);
}
async function init(){try{const s=await request('/api/status');$('diagnostics').textContent=JSON.stringify(s,null,2);window.appStatus=s;preferredModel=s.config?.whisper?.model||'';setTheme(s.config?.ui?.theme==='dark'?'dark':'light');renderInfo(s);log('WhisperGUI v'+s.app_version);log('whisper.cpp: '+s.whisper_version+' (https://github.com/ggml-org/whisper.cpp)');log('FFmpeg: '+s.ffmpeg_version);log('Connected to the local server. Audio files and transcripts are processed locally and are never uploaded to the internet. Model downloads connect to Hugging Face.');if(s.config.languages?.length){const language=$('language');language.replaceChildren(...s.config.languages.map(x=>new Option(x.label,x.code)));}$('vad-enabled').checked=!!s.config.whisper.vad_enabled;$('glossary-enabled').checked=!!s.config.whisper.glossary_enabled;$('glossary-hint').value='';$('carry-initial-prompt').checked=false;syncHintState();$('experimental-features').checked=!!s.config.ui.experimental_features;syncExperimentalFeatures();$('temperature').value=s.config.whisper.temperature||0;syncTemperature();$('package-zip').checked=s.config.output?.package_zip!==false;$('translate').checked=s.config.whisper.translate;$('language').value=s.config.whisper.language||'auto'}catch(e){log('Diagnostics: '+e.message)}await refresh();connectEvents()}init();

function renderInfo(s){const rows=[['WhisperGUI', 'v'+s.app_version],['whisper.cpp',s.whisper_version],['FFmpeg',s.ffmpeg_version],['Installation',s.mode],['Configuration',s.config_file],['Binaries',s.bin_dir],['FFmpeg',s.ffmpeg+(s.ffmpeg_found?' (found)':' (missing)')],['Whisper',s.whisper+(s.whisper_found?' (found)':' (missing)')],['Models',s.models_dir+(s.models_dir_exists?'':' (directory missing)')],['Model management',s.models_admin_managed?'Administrator':'User']];const table=document.createElement('div');table.className='info-table';for(const [label,value] of rows){const l=document.createElement('strong'),v=document.createElement('span');l.textContent=label;if(label==='whisper.cpp'){const link=document.createElement('a');link.href='https://github.com/ggml-org/whisper.cpp';link.target='_blank';link.rel='noopener noreferrer';link.textContent='(GitHub)';v.append(document.createTextNode(value+' '),link)}else if(['Configuration','Binaries','Models'].includes(label)){const path=label==='Models'?s.models_dir:value;v.className='copy-path';v.textContent=value;v.title='Click to copy to clipboard';v.tabIndex=0;v.setAttribute('role','button');v.setAttribute('aria-label','Copy '+label.toLowerCase()+' path');const copy=async()=>{try{await navigator.clipboard.writeText(path);v.title='Copied!';setTimeout(()=>v.title='Click to copy to clipboard',1400)}catch(e){log('Copy failed: '+e.message)}};v.addEventListener('click',copy);v.addEventListener('keydown',e=>{if(e.key==='Enter'||e.key===' '){e.preventDefault();copy()}})}else{v.textContent=value}table.append(l,v)}$('info-content').replaceChildren(table)}
async function refreshInfo(){try{const s=await request('/api/status');window.appStatus=s;$('diagnostics').textContent=JSON.stringify(s,null,2);renderInfo(s)}catch(e){log('Information: '+e.message)}}
$('info').onclick=async()=>{await refreshInfo();$('infodialog').showModal()};document.querySelectorAll('[data-close]').forEach(b=>b.onclick=()=>$(b.dataset.close).close());
let dialogTimer, resetModelProgressOnOpen=false;
function formatBytes(n){return n>0?(n/(1024*1024)).toFixed(1)+' MiB':'Unknown size'}
function modelRow(m,admin){const row=document.createElement('div');row.className='model-item';const installed=m.local_size>0;const indicator=document.createElement(installed?'span':'button'),name=document.createElement('div'),state=document.createElement('span'),buttons=document.createElement('div');indicator.className='model-indicator'+(installed?' model-indicator-installed':'');indicator.textContent=installed?'✓':'';indicator.setAttribute('aria-label',installed?'Downloaded':'Download '+m.label);indicator.title=installed?'Downloaded':'Download '+m.label;if(!installed){indicator.type='button';indicator.disabled=!admin||window.modelBusy;indicator.onclick=()=>operateModel('download',m.id)}buttons.className='buttons';const strong=document.createElement('strong');strong.textContent=m.label;const small=document.createElement('small');small.textContent=m.filename+' · '+formatBytes(m.remote_size||m.local_size);name.append(strong,small);state.className='status';state.textContent=m.status==='absent'?'Not downloaded':m.status==='update'?'Update available':m.status==='installed'?'Installed':'Installed (remote status unavailable)';if(m.status==='absent'||m.status==='update'){const b=document.createElement('button');b.textContent=m.status==='update'?'Update':'Download';b.disabled=!admin||window.modelBusy;b.onclick=()=>operateModel('download',m.id);buttons.append(b)}if(m.local_size>0){const b=document.createElement('button');b.textContent='Delete';b.disabled=!admin||window.modelBusy;b.onclick=()=>{if(confirm('Delete '+m.filename+'?'))operateModel('delete',m.id)};buttons.append(b)}row.append(indicator,name,state,buttons);return row}
async function loadCatalog(){try{const d=await request('/api/model-catalog');$('model-admin').textContent=d.writable?'Models are downloaded from Hugging Face.':'System-wide installation: downloading, updating and deleting models requires administrator privileges.';window.modelBusy=d.busy;const list=$('model-list');list.replaceChildren();const sections=[['Speech Recognition Models',d.models.filter(m=>!m.id.startsWith('silero-vad-'))],['Additional Models',d.models.filter(m=>m.id.startsWith('silero-vad-'))]];for(const [heading,models] of sections){if(!models.length)continue;const title=document.createElement('div');title.className='model-section-title';title.textContent=heading;list.append(title,...models.map(m=>modelRow(m,d.writable)));}}catch(e){$('model-admin').textContent='Error: '+e.message}}
async function openModels(){resetModelProgressOnOpen=true;$('modelsdialog').showModal();await loadCatalog();await checkModelProgress();clearInterval(dialogTimer);dialogTimer=setInterval(checkModelProgress,850)}
$('modelsdialog').addEventListener('close',()=>{clearInterval(dialogTimer);refresh();refreshInfo()});
async function operateModel(action,id){resetModelProgressOnOpen=false;try{await request('/api/model-'+action,'POST',{id});await loadCatalog();await checkModelProgress();await refreshInfo()}catch(e){$('model-message').textContent=e.message}}
$('model-cancel').onclick=async()=>{try{await request('/api/model-cancel','POST')}catch(e){$('model-message').textContent=e.message}};
async function checkModelProgress(){try{const p=await request('/api/model-progress');if(p.busy)resetModelProgressOnOpen=false;const reset=resetModelProgressOnOpen&&!p.busy;$('model-message').textContent=reset?'Ready':(p.message||'Ready');$('model-bar').value=reset?0:Math.max(0,p.percent||0);$('model-percent').textContent=reset?'0%':(p.percent>=0?p.percent.toFixed(1)+'%':'Unknown size');$('model-cancel').disabled=!p.busy;if(window.modelBusy!==p.busy){window.modelBusy=p.busy;await loadCatalog();await refreshInfo();if(!p.busy)await refresh()}}catch(e){$('model-message').textContent=e.message}}
// A dedicated WebSocket tracks this browser tab. Native WS ping/pong works
// even when JavaScript timers are throttled in a background tab.
let clientSocket, clientRetry;
function connectClient(){
  if(exiting)return;
  const proto=location.protocol==='https:'?'wss:':'ws:';
  clientSocket=new WebSocket(proto+'//'+location.host+'/api/client?token='+encodeURIComponent(token));
  clientSocket.onopen=()=>{if(clientRetry){clearTimeout(clientRetry);clientRetry=null}};
  clientSocket.onclose=()=>{if(!exiting)clientRetry=setTimeout(connectClient,1000)};
  clientSocket.onerror=()=>{}; // onclose handles reconnection
}
connectClient();

// Context text is intentionally held only in the current browser session.
function syncHintState(){const enabled=$('glossary-enabled').checked;$('glossary-hint').disabled=!enabled;$('carry-initial-prompt').disabled=!enabled;}
function syncExperimentalFeatures(){const enabled=$('experimental-features').checked;$('textgrid-option').hidden=!enabled;if(!enabled)$('textgrid').checked=false;}
function syncTemperature(){$('temperature-value').textContent=Number($('temperature').value).toFixed(1);}
$('advanced').onclick=()=>{$('glossary-status').textContent='';$('advanceddialog').showModal()};
$('glossary-enabled').onchange=syncHintState;
$('experimental-features').onchange=syncExperimentalFeatures;
$('temperature').oninput=syncTemperature;
$('save-glossary').onclick=async()=>{
 const enabled=$('glossary-enabled').checked,vad_enabled=$('vad-enabled').checked,experimental_features=$('experimental-features').checked,temperature=Number($('temperature').value),package_zip=$('package-zip').checked;
 try{await request('/api/preferences/glossary','POST',{enabled,vad_enabled,experimental_features,temperature,package_zip});$('glossary-status').textContent='Settings saved';}
 catch(e){$('glossary-status').textContent='Save failed: '+e.message}
};

// Exit is explicit: do not rely on window.close(), which most browsers disallow.
$('exit').onclick=async()=>{
 if(exiting)return;
 let active=busy||!!window.modelBusy;
 try {const a=await request('/api/activity');active=a.transcription||a.model_download;}catch(_){/* use known client state */}
 const message=active
  ? 'A transcription or model download is running. Exiting will interrupt it. Exit WhisperGUI?'
  : 'Are you sure you want to exit WhisperGUI?';
 if(!window.confirm(message))return;
 $('exit').disabled=true;
 try {
   await request('/api/shutdown','POST');
   exiting=true;
   if(clientRetry){clearTimeout(clientRetry);clientRetry=null;}
   if(clientSocket)clientSocket.close();
   document.querySelector('main').replaceChildren();
   const message=document.createElement('div');
   message.className='exit-message';
   const heading=document.createElement('h2');heading.textContent='WhisperGUI has closed.';
   const detail=document.createElement('p');detail.textContent='You can safely close this tab.';
   message.append(heading,detail);document.querySelector('main').append(message);
   document.title='WhisperGUI — closed';
   window.close(); // Best effort only; browser tabs normally stay open.
 }catch(e){$('exit').disabled=false;log('Could not close WhisperGUI: '+e.message)}
};
