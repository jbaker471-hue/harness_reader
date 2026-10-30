const fs=require('fs'),vm=require('vm');
const path=require('path');const f=path.join(__dirname,'assets/ui.html');
const html=fs.readFileSync(f,'utf8');const src=html.match(/<script>([\s\S]*?)<\/script>/)[1];
new Function(src);
function block(start,end){return src.slice(src.indexOf(start),src.indexOf(end,src.indexOf(start)));}
const isolate={};vm.createContext(isolate);
vm.runInContext(block('function sortCav(','function resetChecks(')+block('function wv(','function wireToNet('),isolate);
const fixture={wires:[{connector:'P1',cavity:'A',end:'J2',endCavity:'a'},{connector:'P1',cavity:'1',end:'J2',endCavity:'2'}],candidates:[{Connector:'J2',Number:'3'}],cavityModels:[{tag:'J2',all:['a','2','3'],allVerified:true}]};
const equal=(a,b)=>{if(JSON.stringify(a)!==JSON.stringify(b))throw Error(JSON.stringify(a)+' != '+JSON.stringify(b))};
equal(Array.from(isolate.extractedOccupied(fixture,'J2')),['2','3','a']);
equal(Array.from(isolate.extractedOccupied(fixture,'P1')),['1','A']);
equal(isolate.sourceCavityModel(fixture,'j2').allVerified,true);
equal(Array.from(isolate.splitCavs('A, a, 1')),['A','a','1']);
console.log('PASS: UI syntax, far-end occupancy, candidate merging, case-sensitive cavity IDs, verified model lookup');

// Face display must distinguish inferred geometry from verified drawing positions.
const faceContext={st:{raw:{cavityModels:[{tag:'P1',all:['A','a','1'],allVerified:true,views:[{page:1,side:'front',r:100,positions:[{label:'A',x:20,y:0,r:6},{label:'1',x:-20,y:0,r:6},{label:'',x:0,y:30,r:6}]}]}]},nets:[]},sourceCavityModel:isolate.sourceCavityModel,splitCavs:isolate.splitCavs};
vm.createContext(faceContext);
vm.runInContext(block('const faceSides=','function renderConnectors(').replace(/^const faceSides=/,'var faceSides='),faceContext);
const d=faceContext.faceData({tag:'P1',occupied:'A'},'rear');
equal(Array.from(d.missingPositions),['a']);
equal(d.unlabelledPositions,1);
if(!d.mirrored||d.pts[0].x>=200)throw Error('rear mirror did not flip X');
if(!src.includes('ids=[...new Set([...d.pts.map(p=>p.id).filter(Boolean),...d.all])]'))throw Error('missing inventory cavities in details table');
console.log('PASS: synthetic face geometry detection, mirror, missing/unlabeled cavities');

// A source cavity model separates explicit unused rows from conductors.
equal(Array.from(isolate.extractedOccupied({cavityModels:[{tag:'P1',wired:['a'],explicitUnused:[{cavity:'A'}]}],candidates:[{Connector:'P1',Number:'A'}]},'P1')),['a']);
if(!src.includes('!model?.conflicts?.length')||!src.includes('(model?.explicitUnused||[]).map(u=>u.cavity)'))throw Error('build plan must preserve unused declarations and unresolved source conflicts');
console.log('PASS: source model occupancy and build-plan conflict safeguards');

const diagnostics={};vm.createContext(diagnostics);
vm.runInContext(block('function localRecognitionCheck(', 'function showLocalRecognitionCheck('),diagnostics);
equal(Array.from(diagnostics.localRecognitionCheck(null)),['NO_DRAWING: Open a drawing first.']);
const privateFixture={words:42,boms:[],wires:[],connectors:[{Name:'UNASSIGNED',Part:'PRIVATE_TEST_VALUE'}],filename:'PRIVATE_TEST_VALUE',engineError:'PRIVATE_TEST_VALUE'};
const messages=diagnostics.localRecognitionCheck(privateFixture).join('\n');
if(messages.includes('PRIVATE_TEST_VALUE'))throw Error('diagnostic leaked source content');
for(const code of ['ENGINE_ERROR','TEXT_PRESENT','BOM_UNRECOGNIZED','CONDUCTORS_UNRECOGNIZED','ASSOCIATION_UNRESOLVED'])if(!messages.includes(code))throw Error('missing diagnostic '+code);
if(!diagnostics.localRecognitionCheck({words:0}).join('').includes('NO_TEXT'))throw Error('missing no-text diagnostic');
console.log('PASS: local recognition checks and fixed-message privacy boundary');

const labor={};vm.createContext(labor);
vm.runInContext(block('const laborOps=', 'function laborFields('),labor);
const counts={cutkit:0,crimp:10,solder:0,shield:0,backshell:0,label:0,test:0},rates={cutkit:0,crimp:6,solder:0,shield:0,backshell:0,label:0,test:0};
const target={counts,rates,qty:3,setup:2};
let e=labor.laborEstimate(target,[]);equal(e.hours,5);
e=labor.laborEstimate(target,[{counts,qty:2,setup:1,estimated:9,actual:5}]);equal(e.hours,8);equal(e.basis,'actual job hours');
e=labor.laborEstimate(target,[{counts,qty:2,setup:1,estimated:9,actual:null}]);equal(e.hours,14);equal(e.basis,'prior estimates only');
e=labor.laborEstimate(target,[{counts,qty:2,setup:1,estimated:9,actual:null},{counts,qty:2,setup:1,actual:5}]);equal(e.hours,8);equal(e.references,1);
for(const bad of [{...target,qty:0},{...target,setup:-1},{...target,counts:{...counts,crimp:-1}},{...target,rates:{...rates,crimp:0}}]){
 let rejected=false;try{labor.laborEstimate(bad,[])}catch{rejected=true}if(!rejected)throw Error('invalid labor input accepted');
}
console.log('PASS: labor setup/quantity arithmetic, actual-vs-estimate precedence and invalid-input rejection');

const totals={};vm.createContext(totals);
vm.runInContext(block('const laborFeatures=', 'function laborTotalAdd('),totals);
const ref={mode:'total',name:'Invented assembly',scope:'Assembly and test',hoursBasis:'unit',qty:1,estimated:7,actual:null};
equal(totals.laborTotalEstimate(ref,3,true).hours,21);
equal(totals.laborTotalEstimate({...ref,hoursBasis:'batch',qty:4,estimated:28},2,true).hours,14);
equal(totals.laborTotalEstimate({...ref,actual:8},2,true).hours,16);
equal(totals.laborTotalEstimate(ref,1,true).basis,'estimated hours');
for(const bad of [{...ref,estimated:0},{...ref,estimated:NaN},{...ref,actual:-1},{...ref,qty:0},{...ref,qty:1.5},{...ref,hoursBasis:'unknown'},{...ref,estimated:null}]){
 let rejected=false;try{totals.laborTotalReference(bad)}catch{rejected=true}if(!rejected)throw Error('Invalid total-hours reference accepted');
}
for(const [q,confirmed] of [[0,true],[1.5,true],[2,false]]){
 let rejected=false;try{totals.laborTotalEstimate(ref,q,confirmed)}catch{rejected=true}if(!rejected)throw Error('Invalid target accepted');
}
console.log('PASS: total-hours guesses without operation rates, per-unit/batch normalization, actual precedence and comparison confirmation');

(async()=>{
 const nodes={};
 function node(){return {value:'',checked:false,textContent:'',children:[],appendChild(x){this.children.push(x)}}}
 const ctx={$:id=>nodes[id]||(nodes[id]=node()),document:{createElement:node}};
 vm.createContext(ctx);vm.runInContext(block('const laborOps=','async function runBuildPlan('),ctx);
 const data={version:2,references:[ref,{name:'Invented operations',counts,rates,qty:2,setup:1,estimated:9,actual:null}]};
 await ctx.laborImport({size:100,text:async()=>JSON.stringify(data)});
 equal(vm.runInContext('laborRefs.length',ctx),2);
 equal(vm.runInContext('laborTotalReference(laborRefs[0]).unit',ctx),7);
 equal(nodes.laborTotalOutput.textContent,'Reference file loaded locally.');
 await ctx.laborImport({size:100,text:async()=>JSON.stringify({version:2,references:[{...ref,estimated:-1}]})});
 equal(vm.runInContext('laborRefs.length',ctx),2);
 if(!nodes.laborTotalOutput.textContent.startsWith('Import failed:'))throw Error('Import failure not visible');
 ctx.laborRemoveReference(0);equal(vm.runInContext('laborRefs.length',ctx),1);
 await ctx.laborImport({size:100,text:async()=>JSON.stringify({version:1,references:[data.references[1]]})});
 equal(vm.runInContext('laborRefs.length',ctx),1);
 equal(nodes.laborTotalOutput.textContent,'Reference file loaded locally.');
 console.log('PASS: mixed reference import, legacy compatibility, atomic rejection and removal');
})().catch(e=>{console.error(e);process.exitCode=1});

const featureA={conductors:10,connectors:2,branches:1,sheathingFeet:0,overbraidFeet:0,floatingShields:2,terminatedShields:2,labels:4,backshells:2,blankPins:12,sealingPlugs:12};
const featureB={...featureA,conductors:20,sheathingFeet:6,overbraidFeet:4,terminatedShields:6,labels:8};
const refs=[{...ref,features:featureA,featuresReviewed:true,fillerTimeBasis:'included'},{...ref,name:'Invented second',estimated:13,features:featureB,featuresReviewed:true,fillerTimeBasis:'included'}];
let match=totals.laborFeatureEstimate(featureA,true,2,true,refs);
equal(match.hours,14);equal(match.matches.length,1);
const midway={};for(const k of Object.keys(featureA))midway[k]=(featureA[k]+featureB[k])/2;
match=totals.laborFeatureEstimate(midway,true,1,true,refs);
if(Math.abs(match.hours-10)>1e-8)throw Error('Symmetric feature interpolation failed');
equal(match.basis,'estimated hours');
match=totals.laborFeatureEstimate(featureA,true,1,true,[{...refs[0],actual:8}]);equal(match.hours,8);equal(match.basis,'actual hours');
for(const [f,reviewed,scope,pool] of [[{...featureA,labels:null},true,true,refs],[featureA,false,true,refs],[featureA,true,false,refs],[{...featureA,conductors:30},true,true,refs],[featureA,true,true,[ref]]]){
 let rejected=false;try{totals.laborFeatureEstimate(f,reviewed,1,scope,pool)}catch{rejected=true}if(!rejected)throw Error('Unsupported construction estimate accepted');
}
console.log('PASS: construction matching, shield/material/label features, unknown-vs-zero, evidence requirements and extrapolation holds');

equal(totals.laborFillerHours({blankPins:1,sealingPlugs:1}),1/120);
if(Math.abs(totals.laborFeatureEstimate(featureA,true,3,true,refs).fillerHours-.3)>1e-8)throw Error('Filler quantity multiplier failed');
const excluded={...refs[0],estimated:7,fillerTimeBasis:'excluded'};
if(Math.abs(totals.laborTotalReference(excluded).unit-7.1)>1e-8)throw Error('Excluded filler allowance not added');
equal(totals.laborTotalReference(refs[0]).unit,7);
if(Math.abs(totals.laborFeatureEstimate(featureA,true,1,true,[excluded]).hours-7.1)>1e-8)throw Error('Filler allowance counted twice');
const includedBatch={...refs[0],hoursBasis:'batch',qty:2,estimated:14};
equal(totals.laborTotalReference(includedBatch).unit,7);
let fillerRejected=false;try{totals.laborTotalReference({...refs[0],estimated:.05})}catch{fillerRejected=true}if(!fillerRejected)throw Error('Negative residual work accepted');
console.log('PASS: 15 seconds per blank pin and sealing plug, included/excluded totals, batch normalization and no double counting');

const moreFillers={...featureA,blankPins:60,sealingPlugs:60};
if(Math.abs(totals.laborFeatureEstimate(moreFillers,true,2,true,[refs[0]]).hours-14.8)>1e-8)throw Error('Known filler rate must apply independently of comparable construction range');
console.log('PASS: independently specified filler rate applies to target quantities outside reference filler counts');

const bomImport={};vm.createContext(bomImport);
vm.runInContext(block('function bomObj(', 'function asRequired('),bomImport);
const imported=bomImport.bomObj({item:'1',part:'EXAMPLE-101',partReference:'LOCAL-501',qty:'2'});
equal(imported.part,'EXAMPLE-101');equal(imported.partReference,'LOCAL-501');equal(imported.notes,'Secondary part reference: LOCAL-501');
console.log('PASS: secondary BOM part references remain separate and visible in notes');

const reviewed={};vm.createContext(reviewed);
vm.runInContext(block('function reviewedDrawingLabor(', 'function laborUseDrawing(')+block('function preparationRows(', 'function laborTotalReference('),reviewed);
const state={sourceMatched:true,raw:{branches:['CB-P3']},connectors:[{verified:true},{verified:true}],nets:[{reviewed:true,endpoints:'P8:A, J9:a'}]};
equal(reviewed.reviewedDrawingLabor(state,true),{conductors:1,connectors:2,branches:1});
for(const bad of [{...state,sourceMatched:false},{...state,nets:[{reviewed:false}]},{...state,nets:[...state.nets,...state.nets]},{...state,nets:[{reviewed:true,endpoints:'P8:A, @UNRESOLVED'}]}]){
 let rejected=false;try{reviewed.reviewedDrawingLabor(bad,true)}catch{rejected=true}if(!rejected)throw Error('Unsafe automatic drawing counts');
}
equal(reviewed.preparationRows({connectors:[{tag:'P8',unusedComplete:false,unused:['A'],kit:[]} ]})[0].cavities,'HELD');
equal(reviewed.preparationRows({connectors:[{tag:'P8',unusedComplete:true,unused:['A','a'],issues:[],kit:[]} ]})[0].cavities,'A, a');
console.log('PASS: reviewed drawing count transfer and preparation holds');

if(src.includes("part:'UNRESOLVED COTS / BRANCH END'"))throw Error('branch endpoint is still promoted to a connector');
if(!src.includes('branches:[...new Set(d.branches||[])]'))throw Error('branch endpoint evidence is not retained separately');
console.log('PASS: branch endpoints remain evidence and do not inflate connector count');

const govNodes={};const govAlerts=[];
const govNode=()=>({value:'',checked:false,disabled:false,textContent:'',innerHTML:'',className:''});
const governance={
 st:{raw:{graphicalCallouts:[
  {kind:'note definition candidate',reference:'7',matches:['APPLY SEALANT TO THE IDENTIFIED TERMINATION'],page:2,box:{x0:10,y0:20,x1:30,y1:40}},
  {kind:'note reference candidate',reference:'7',matches:['APPLY SEALANT TO THE IDENTIFIED TERMINATION'],page:1,box:{x0:50,y0:60,x1:70,y1:80},definitionPage:2,definitionBox:{x0:10,y0:20,x1:200,y1:50}}
 ]},connectors:[],bom:[{item:'4',part:'ACME-101',description:'PURCHASED MODULE',vendor:'ACME',page:3}],notes:[],cots:[],lookupQueue:[],evidenceLedger:[],securityPolicy:{classification:'restricted',egress:'blocked'}},
 $:id=>govNodes[id]||(govNodes[id]=govNode()),safe:s=>String(s??''),alert:m=>govAlerts.push(m),csv(){},showReport(){},viewEvidence(){}
};
vm.createContext(governance);vm.runInContext(block('// Source-grounded note, supplier-item and controlled lookup registries.','// Window lifecycle:'),governance);
governance.importDetectedNotes();equal(vm.runInContext('st.notes.length',governance),1);equal(vm.runInContext('st.notes[0].references.length',governance),1);
governance.seedCOTSFromBOM();equal(vm.runInContext('st.cots[0].itemClass',governance),'Unresolved — review');
governance.$('lookupPart').value='ACME-101';governance.$('lookupProvider').value='manufacturer';governance.$('releaseAuthority').checked=true;
governance.queueLookup();equal(vm.runInContext('st.lookupQueue.length',governance),0);
vm.runInContext("st.securityPolicy={classification:'public',egress:'part-only'}",governance);governance.queueLookup();equal(vm.runInContext('st.lookupQueue.length',governance),1);
equal(vm.runInContext('st.lookupQueue[0].payload.drawingContextIncluded',governance),false);
equal(Array.from(vm.runInContext('Object.keys(st.lookupQueue[0].payload)',governance)),['partNumber','provider','requestedFields','drawingContextIncluded']);
console.log('PASS: notes retain definition/reference locations, COTS is never inferred, and restricted lookup is blocked');

const bandTooling={
 unique:a=>[...new Set(a)],
 st:{
  raw:{pdfText:'',assemblyFeatures:[]},connectors:[],
  bom:[
   {item:'3',part:'M85049/82-25N',partReference:'M85049/82-25N',description:'BANDING BACKSHELL'},
   {item:'10',part:'600-052',partReference:'HDW-470',description:'BAND, 0.240 IN'}
  ],
  assemblyRefs:[{kind:'band',matches:['600-052','600-052-1'],class:'Standard shield termination band',requirement:'Glenair 600-058',source:'manufacturer',status:'source-review',specification:'M81306/1A',alternates:['DMC DBS-2100'],alternateSource:'DMC manufacturer',alternateStatus:'qualified-equivalent-by-shared-specification'}]
 }
};
vm.createContext(bandTooling);
vm.runInContext(block('function recognizedWireCableRows(', 'function renderAssemblyRefs(')+block('function inferredContactForConnector(', 'function allRequirementRows('),bandTooling);
let bandRows=bandTooling.requiredAssemblyRows().filter(x=>x.operation==='Shield-band installation');
if(!bandRows.some(x=>x.item==='600-052'&&x.tool.includes('600-058')))throw Error('exact band did not select its referenced tool');
if(!bandRows.some(x=>x.item==='600-052'&&x.tool.includes('DMC DBS-2100')&&x.tool.includes('M81306/1A')))throw Error('qualified DMC alternate and governing specification were not shown');
if(!bandRows.some(x=>x.item==='M85049/82-25N'&&x.tool.includes('600-052')&&x.status==='VERIFY BAND APPLICATION'))throw Error('banded backshell was not linked to the exact band/tool pair');
bandTooling.st.bom=bandTooling.st.bom.slice(0,1);
bandRows=bandTooling.requiredAssemblyRows().filter(x=>x.operation==='Shield-band installation');
if(!bandRows.some(x=>x.status==='BAND PART UNRESOLVED'&&x.tool.includes('exact band P/N required')))throw Error('banded backshell without a band must remain unresolved');
console.log('PASS: exact shield-band tooling and unresolved banded-backshell safeguards');

const backshellTooling={
 unique:a=>[...new Set(a)],
 st:{raw:{pdfText:'',assemblyFeatures:[]},connectors:[],assemblyRefs:[],bom:[
  {item:'1',part:'M85049/20-25N',description:'BANDING BACKSHELL'},
  {item:'2',part:'M85049/38S25W',description:'STRAIN RELIEF CLAMP'},
  {item:'3',part:'TXR54AB00-2216AI',description:'TINEL-LOCK BACKSHELL'},
  {item:'4',part:'203M0XX',description:'FACTORY-ATTACHED BRAID WITH MAGNEFORM CRIMP RING'}
 ]}
};
vm.createContext(backshellTooling);
vm.runInContext(block('function recognizedWireCableRows(', 'function renderAssemblyRefs(')+block('function inferredContactForConnector(', 'function allRequirementRows('),backshellTooling);
const terminationRows=backshellTooling.requiredAssemblyRows();
if(!terminationRows.some(x=>x.item==='M85049/20-25N'&&x.operation==='Crimp-ring shield termination'&&x.status==='CRIMP TOOL UNRESOLVED'))throw Error('M85049/20 crimp-ring tooling hold missing');
if(terminationRows.some(x=>x.item==='M85049/20-25N'&&x.operation==='Shield-band installation'))throw Error('M85049/20 misleading description incorrectly produced a banding-backshell requirement');
if(!terminationRows.some(x=>x.item==='M85049/38S25W'&&x.operation==='Backshell clamp fastening'&&x.tool.includes('flat-blade')))throw Error('clamp backshell flat-blade driver requirement missing');
if(!terminationRows.some(x=>x.item==='TXR54AB00-2216AI'&&x.operation==='Tinel-Lock ring installation'&&x.tool.includes('AD-5000')))throw Error('Tinel-Lock tool requirement missing');
if(terminationRows.some(x=>x.item==='203M0XX'&&x.status==='CRIMP TOOL UNRESOLVED'))throw Error('factory-preterminated braid incorrectly demands a field crimp tool');
console.log('PASS: clamp, crimp-ring, Tinel-Lock, and factory-preterminated backshell tooling are distinct');

const wireTooling={
 unique:a=>[...new Set(a)],
 st:{raw:{pdfText:'',assemblyFeatures:[]},connectors:[],assemblyRefs:[],toolEquivalences:[],bom:[{item:'1',part:'M22759/16-22-9',description:'PRIMARY WIRE'}],wireCableRefs:[{pattern:'\\b(?:M|AS)?22759/(?:16|17)-[0-9][0-9]?(?:-[0-9][0-9]?)?\\b',family:'AS22759 medium-weight wire',construction:'Modified ETFE',preparation:'Resolve exact OD and strip process',termination:'Confirm conductor dimensions and contact barrel',test:'Continuity and workmanship inspection',source:'manufacturer',status:'FAMILY MATCH — EXACT SLASH SHEET AND OD REQUIRED'}]}
};
vm.createContext(wireTooling);
vm.runInContext(block('function recognizedWireCableRows(', 'function renderAssemblyRefs(')+block('function inferredContactForConnector(', 'function allRequirementRows('),wireTooling);
const wireRows=wireTooling.requiredAssemblyRows().filter(x=>x.item==='M22759/16-22-9');
if(wireRows.length!==3||!wireRows.some(x=>x.operation==='Wire/cable preparation'&&x.tool.includes('exact OD'))||!wireRows.some(x=>x.operation==='Wire/cable inspection and test'))throw Error('wire family did not generate preparation, termination, and test requirements');
wireTooling.st.toolEquivalences=[{specification:'AS22520/2',primary:'DMC AFM8 + exact positioner',alternates:[['DMC WA22 pneumatic'],['DMC AFME8B battery']],conditions:'exact positioner and application required',sourceAlternate:'DMC',status:'ALTERNATIVE POWER ROUTE'}];
const eq=wireTooling.toolEquivalenceForText('AFM8 + K13-1');
if(!eq||eq.specification!=='AS22520/2')throw Error('AFM8 documented powered-frame alternatives were not matched');
console.log('PASS: wire construction requirements and documented crimp-frame alternatives');

// Workflow navigation and job-status summary.
for(const g of ['1 · Inspect the drawing','2 · Quote','3 · Build'])if(!html.includes('<div class="navgroup">'+g+'</div>'))throw Error('missing nav group '+g);
const navIds=[...html.matchAll(/class="nav[^"]*" data-id="(\w+)"/g)].map(m=>m[1]);
for(const t of [...html.matchAll(/<section class="tab[^"]*" id="tab-(\w+)"/g)].map(m=>m[1]))if(!navIds.includes(t))throw Error('tab without navigation: '+t);
const jobCtx={$:id=>jobCtx.els[id]||(jobCtx.els[id]={}),els:{},safe:s=>String(s),tab(){},gates:()=>[['a',true,''],['b',false,'']],
 st:{raw:{issues:[{Severity:'ERROR'},{severity:'WARN'}],wires:[{}],boms:[]},connectors:[{verified:true},{verified:false}]}};
vm.createContext(jobCtx);
vm.runInContext(block('function renderJobStatus(','function render()'),jobCtx);
jobCtx.renderJobStatus();
const jh=jobCtx.els.jobStatus.innerHTML;
if(!jh.includes('1 error(s) and 1 other')||!jh.includes('2 found, 1 confirmed')||!jh.includes('No BOM recognized')||!jh.includes('1 of 2 verification checks'))throw Error('job status summary wrong: '+jh);
jobCtx.st.raw=null;jobCtx.renderJobStatus();
if(!jobCtx.els.jobStatus.innerHTML.includes('No drawing loaded'))throw Error('empty job status');
console.log('PASS: workflow navigation covers every tab; job status summarizes drawing state and empty state');

// Cut list and quote.
const cutCtx={safe:s=>String(s),$:()=>null};vm.createContext(cutCtx);
vm.runInContext(block('/*CUTLIST-START*/','function renderCutList(').replace('const LENGTH_UNITS','var LENGTH_UNITS')+block('function freshQuote(','function quoteInput('),cutCtx);
const near=(a,b,m)=>{if(Math.abs(a-b)>1e-6)throw Error((m||'value')+': '+a+' != '+b)};
const pl=cutCtx.parseLength;
near(pl('12.5','IN').inches,12.5,'plain inches');near(pl('3 FT','IN').inches,36,'feet suffix');near(pl('300 mm','IN').inches,300/25.4,'mm suffix');near(pl('12 1/2','IN').inches,12.5,'mixed fraction');
near(pl('2','FT').inches,24,'default unit applies');if(!pl('2','FT').assumed||pl('2 IN','FT').assumed)throw Error('assumed-unit flag wrong');
for(const bad of ['abc','0','-3','1/0','12..5','12 XX'])if(pl(bad,'IN').inches!==null||!pl(bad,'IN').invalid)throw Error('should be invalid: '+bad);
if(pl('','IN').inches!==null||pl('','IN').invalid)throw Error('blank length must be missing, not invalid');
const cl=cutCtx.buildCutList([
 {id:'N1',endpoints:'P1:1, J2:4',wire:'w1',awg:'22',color:'red',length:'12.5'},
 {id:'N2',endpoints:'J2:4, P1:1',wire:'W1',awg:'22',color:'RED',length:'12.5'},          // same wire from the other end: merge
 {id:'N3',endpoints:'P1:2, J2:5',wire:'W2',awg:'22',color:'BLK'},                           // no length
 {id:'N4',endpoints:'P1:3, J2:6',wire:'W3',awg:'20',color:'WHT',length:'10'},{id:'N5',endpoints:'P1:3, J2:6',wire:'W3',awg:'20',color:'WHT',length:'11'}, // conflict
 {id:'N6',endpoints:'P1:4, J2:7',awg:'24',color:'GRN',length:'18 IN'},                      // no wire id: keyed by endpoints
 {id:'N7',endpoints:'P1:5',wire:'W9'},                                                         // single endpoint: not a conductor
 {id:'N8',endpoints:'P1:6, J2:8',wire:'W5',awg:'22',color:'BLU',length:'x'}
],{defUnit:'IN',allowance:1});
equal(cl.rows.length,5);equal(cl.skipped,1);
const byWire=Object.fromEntries(cl.rows.map(r=>[r.wire,r]));
equal([byWire.W1.status,byWire.W1.sources,byWire.W1.cut],['UNIT ASSUMED',2,13.5]);
equal(byWire.W2.status,'NO LENGTH');equal(byWire.W2.cut,null);
equal(byWire.W3.status,'CONFLICTING LENGTHS');equal(byWire.W3.cut,null);
equal(byWire.W5.status,'INVALID LENGTH');
equal(cl.rows.find(r=>r.awg==='24').cut,19);
equal([cl.counts.total,cl.counts.ok,cl.counts.missing,cl.counts.conflict,cl.counts.invalid],[5,2,1,1,1]);
const g22=cl.groups.find(g=>g.label==='22 · RED');near(g22.inches,13.5,'group inches');
if(cl.groups.find(g=>g.label==='22 · BLK').unmeasured!==1)throw Error('unmeasured conductor not counted');
const clash=cutCtx.buildCutList([{endpoints:'A:1, B:1',wire:'W1',awg:'22',color:'RED',length:'5 IN'},{endpoints:'A:1, B:1',wire:'W1',awg:'24',color:'RED',length:'5 IN'}],{}).rows[0];
if(!clash.notes.some(n=>/awg/.test(n)))throw Error('AWG disagreement between merged sources was not flagged');
// quote
const cutOK=cutCtx.buildCutList([{endpoints:'P1:1, J2:1',wire:'W1',awg:'22',color:'RED',length:'120 IN'}],{});
const quoteBase={quote:Object.assign(cutCtx.freshQuote(),{rate:'50',laborHours:'2',scrap:'10',markup:'20',wireCost:{'22 · RED':'0.5'},bomCost:{'1|M39029/58-360|0':'0.4'}}),qty:3,cut:cutOK,
 bom:[{item:'1',part:'M39029/58-360',description:'CONTACT',qty:'2'},{item:'2',part:'M22759/16-22-9',description:'WIRE, 22 AWG',qty:'A/R'}],laborUnitHours:null,drawingErrors:0};
let qr=cutCtx.computeQuote(quoteBase);
near(qr.wireLines[0].buyFt,10*3*1.1,'wire feet incl scrap');near(qr.wireTotal,33*0.5,'wire cost');near(qr.partsTotal,2*3*0.4,'parts cost');near(qr.labor,2*50*3,'labor');
near(qr.total,(16.5+2.4+300)*1.2,'total with markup');equal(qr.blockers,[]);equal(qr.bomSkippedWire,1);
const miss=cutCtx.computeQuote(Object.assign({},quoteBase,{quote:cutCtx.freshQuote(),drawingErrors:2,cut:cl}));
for(const re of [/no length/,/conflicting/,/unreadable/,/\$\/ft/,/unit cost/,/Labor hours/,/Labor rate/,/2 drawing error/])if(!miss.blockers.some(b=>re.test(b)))throw Error('missing quote blocker '+re);
if(miss.complete)throw Error('quote with blockers must not be complete');
const usesLabor=cutCtx.computeQuote(Object.assign({},quoteBase,{quote:Object.assign(cutCtx.freshQuote(),{rate:'10',wireCost:{'22 · RED':'1'},bomCost:{'1|M39029/58-360|0':'1'}}),laborUnitHours:1.5}));
near(usesLabor.hours,1.5,'labor tab hours used when field blank');
const nonNum=cutCtx.computeQuote(Object.assign({},quoteBase,{bom:[{item:'1',part:'X',description:'CAP',qty:'A/R'}]}));
if(!nonNum.blockers.some(b=>/not a plain number/.test(b)))throw Error('A/R BOM quantity must be a blocker, not priced');
console.log('PASS: length parsing and units, cut-list merge/conflict/missing handling, quote totals and blockers');

// Multi-conductor cable: a twisted shielded pair is one cut and one cost, not two.
const pairNets=[];
[['8','15'],['9','16'],['10','18']].forEach(([a,b],i)=>{
 pairNets.push({id:'WIRE-'+(i*2+1),endpoints:'GSE-P3:'+a+', CB-P3:'+a,awg:'24',color:'WHT',length:'30 IN'});
 pairNets.push({id:'WIRE-'+(i*2+2),endpoints:'GSE-P3:'+b+', CB-P3:'+b,awg:'24',color:'BLU',length:'30 IN'});
});
pairNets.push({id:'WIRE-7',endpoints:'GSE-P3:1, CB-P3:1',awg:'22',color:'WHT',length:'10 IN'});
const detected=cutCtx.detectCableSpecs([{item:'5',part:'M27500-24TG2T14',description:'CABLE, TWISTED SHIELDED PAIR, 24 AWG'},{item:'6',part:'D38999/26FJ35SN',description:'CONNECTOR'}]);
equal(cutCtx.effectiveCables(detected,{}),{'24':2});
const single=cutCtx.buildCutList(pairNets,{cables:{}});
equal(single.counts.total,7);                                           // today's doubled behavior when no cable is known
const cabled=cutCtx.buildCutList(pairNets,{cables:cutCtx.effectiveCables(detected,{})});
equal([cabled.counts.total,cabled.counts.cables],[4,3]);                // 3 cables + the lone 22 AWG wire
const g24=cabled.groups.find(g=>g.label==='24 · 2-conductor cable');
equal([g24.count,g24.inches/12],[3,7.5]);                              // 3 cables x 30 in, not 6 x 30 in
const cab=cabled.rows.find(r=>r.cable);
if(!cab.notes.some(n=>/confirm against the drawing/.test(n)))throw Error('cable grouping must be flagged as inferred');
equal([cab.awg,cab.color,cab.conductors],['24','WHT + BLU',2]);
if(cab.nets.length!==2)throw Error('cable must keep both conductor nets so a typed length reaches both');
equal(cabled.rows.find(r=>r.awg==='22').cable,undefined);
// repeated colors, leftovers and user override
const bad=cutCtx.buildCutList([{id:'A',endpoints:'X:1, Y:1',awg:'24',color:'WHT'},{id:'B',endpoints:'X:2, Y:2',awg:'24',color:'WHT'},{id:'C',endpoints:'X:3, Y:3',awg:'24',color:'BLU'}],{cables:{'24':2}});
equal(bad.counts.cables,0);
if(!bad.rows.every(r=>r.notes.some(n=>/pairing uncertain|cable incomplete/.test(n))))throw Error('ungroupable conductors must say why');
equal(cutCtx.effectiveCables(detected,{'24':'1'}),{'24':1});            // user can force plain single wire
equal(cutCtx.effectiveCables(detected,{'24':''}),{'24':2});             // blank keeps the BOM reading
for(const [txt,n] of [['TWISTED SHIELDED PAIR',2],['CABLE 3 CONDUCTOR SHIELDED',3],['SHIELDED CABLE 4C',4]]){const p=cutCtx.parseCableLine({part:'M27500-X',description:txt+' 22 AWG'});if(!p||p.n!==n)throw Error('cable text not read: '+txt)}
equal(cutCtx.parseCableLine({part:'M27500-22TG3T14',description:'WIRE'}).n,3);
if(cutCtx.parseCableLine({part:'D38999/26FJ35SN',description:'CONNECTOR'})!==null)throw Error('connectors must not read as cable');
const amb=cutCtx.detectCableSpecs([{part:'M27500-24TG2T14',description:'CABLE'},{part:'M27500-24TG3T14',description:'CABLE'}]);
if(!amb.get('24').ambiguous||cutCtx.effectiveCables(amb,{})['24']!==undefined)throw Error('conflicting BOM cable lines must not be guessed');
console.log('PASS: multi-conductor cables are one cut and one cost, flagged as inferred, with BOM detection and user override');

// Decoded wire/cable cards: sources are clickable only when https, everything is escaped, unresolved is visible.
cutCtx.safe=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'})[c]);
const card=cutCtx.decodeCardHtml({part:'M27500-22<b>',recognized:true,family:'M27500',familyStatus:'SUPPLIED REFERENCE — UNVERIFIED',familySources:['https://www.wiremasters.com/guides/m27500-anatomy-guide','javascript:alert(1)'],
 fields:[{label:'Shield',value:'UNRESOLVED',code:'Q',status:'CODE NOT IN REFERENCE LIBRARY'},{label:'Outer jacket',value:'White PTFE',code:'06',sources:['https://www.wiremasters.com/guides/m27500-anatomy-guide','http://plain.example/x'],status:'SUPPLIED REFERENCE — UNVERIFIED'}],unresolved:['Shield code Q is not in the reference library.'],notes:['Twist is not stated']});
if(card.includes('javascript:')||card.includes('http://plain')||card.includes('22<b>'))throw Error('decode card leaked an unsafe link or unescaped text');
if(!card.includes('href="https://www.wiremasters.com/guides/m27500-anatomy-guide"')||!card.includes('rel="noopener noreferrer"'))throw Error('https source link missing');
if(!card.includes('Unresolved: Shield code Q')||!card.includes('UNVERIFIED'))throw Error('unresolved/unverified status not shown');
if(!cutCtx.decodeCardHtml({part:'X',recognized:false}).includes('Nothing was assumed'))throw Error('unrecognized part must say nothing was assumed');
if(!cutCtx.decodeCardHtml({part:'X',error:true}).includes('Nothing is assumed'))throw Error('lookup failure must be visible');
console.log('PASS: wire decode cards show sources and status, link only https, escape text, and surface unresolved codes');

// Cable construction: shield termination is required, recorded per end, and never guessed.
const cabRow={cable:true,key:'C:A+B'};
const shielded={shieldPresent:'yes'},unshielded={shieldPresent:'no'};
equal(cutCtx.shieldTermStatus(cabRow,shielded,{}),{needed:true,a:'',b:'',complete:false});
equal(cutCtx.shieldTermStatus(cabRow,shielded,{'C:A+B':{a:'Backshell',b:''}}).complete,false);
equal(cutCtx.shieldTermStatus(cabRow,shielded,{'C:A+B':{a:'Backshell',b:'Ground point'}}).complete,true);
equal(cutCtx.shieldTermStatus(cabRow,unshielded,{}).needed,false);
equal(cutCtx.shieldTermStatus(cabRow,{shieldPresent:'unknown'},{}).needed,false);   // unknown shield: nothing is assumed either way
equal(cutCtx.shieldTermStatus({cable:false,key:'W:1'},shielded,{}).needed,false);   // single wires carry no cable shield
equal(cutCtx.shieldTermStatus(cabRow,null,{}).needed,false);
const consHtml=cutCtx.constructionHtml({signalConductors:2,twist:{value:'',status:'NOT STATED'},groups:{value:'1 pair',status:'STATED IN BOM TEXT'},shieldPresent:'yes',overallShield:{value:'Single silver-coated copper braid',status:'DECODED FROM PART NUMBER'},individualShields:{value:'',status:'NOT STATED'},drain:{value:'',status:'NOT STATED'},notes:['CONFLICT: pair count <x> mismatch','The shield is not a signal conductor.']});
if((consHtml.match(/NOT STATED/g)||[]).length!==3)throw Error('unstated attributes must read NOT STATED, not default');
if(!consHtml.includes('CONFLICT</span>')||consHtml.includes('<x>'))throw Error('conflict not surfaced or text not escaped');
const shieldBlock=cutCtx.computeQuote(Object.assign({},quoteBase,{shieldOpen:2}));
if(!shieldBlock.blockers.some(b=>/2 shielded cable\(s\) have no shield termination/.test(b)))throw Error('open shield terminations must block the quote');
if(cutCtx.computeQuote(Object.assign({},quoteBase,{shieldOpen:0})).blockers.some(b=>/shield/.test(b)))throw Error('no shield blocker expected when all are recorded');
console.log('PASS: cable construction display, shield termination tracking and quote blocker');

// Overall harness length from the drawing: used only for conductors with no length of their own.
const hNets=[{id:'A',endpoints:'P1:1, J1:1',wire:'W1',awg:'22',color:'RED'},{id:'B',endpoints:'P1:2, J1:2',wire:'W2',awg:'22',color:'BLK',length:'30 IN'},{id:'C',endpoints:'P1:3, J1:3',wire:'W3',awg:'22',color:'WHT',length:'bad'}];
const hl=cutCtx.buildCutList(hNets,{allowance:2,harness:{inches:48,assumed:false,text:'48 IN'}});
const hw=Object.fromEntries(hl.rows.map(r=>[r.wire,r]));
equal([hw.W1.status,hw.W1.cut],['HARNESS LENGTH',50]);
equal([hw.W2.status,hw.W2.cut],['OK',32]);                                   // its own length always wins
equal(hw.W3.status,'INVALID LENGTH');                                         // an unreadable length is never replaced silently
if(!hw.W1.notes.some(n=>/overall harness length/.test(n)))throw Error('harness-derived cut must say so');
equal([hl.counts.harness,hl.counts.ok,hl.counts.invalid,hl.counts.missing],[1,2,1,0]);
const noH=cutCtx.buildCutList(hNets,{allowance:2});
equal(noH.rows.find(r=>r.wire==='W1').status,'NO LENGTH');                    // no harness length: nothing invented
const unitless=cutCtx.buildCutList([hNets[0]],{allowance:0,harness:{inches:48,assumed:true,text:'48'}}).rows[0];
equal(unitless.status,'UNIT ASSUMED');                                          // an unstated unit is still flagged
const hCable=cutCtx.buildCutList([{id:'A',endpoints:'P1:1, J1:1',awg:'24',color:'WHT'},{id:'B',endpoints:'P1:2, J1:2',awg:'24',color:'BLU'}],{allowance:2,cables:{'24':2},harness:{inches:48,assumed:false,text:'48 IN'}});
equal([hCable.rows.length,hCable.rows[0].status,hCable.rows[0].cut,hCable.groups[0].inches],[1,'HARNESS LENGTH',50,50]);   // one cable, one 50 in cut
// candidate selection and stale state
cutCtx.st={raw:{harnessLengths:[{page:1,label:'HARNESS LENGTH',text:'48 IN'},{page:2,label:'OVERALL LENGTH',text:'48 IN'}]},harnessLength:null,harnessLengthOff:false,quote:{wireCost:{a:1},bomCost:{b:2}},shieldTerm:{x:1},cableSpec:{'24':'2'}};
cutCtx.autoHarness();equal(cutCtx.st.harnessLength.text,'48 IN');             // one distinct value on two sheets: pre-selected for confirmation
cutCtx.st.harnessLength=null;cutCtx.st.raw.harnessLengths.push({page:2,label:'CABLE LENGTH',text:'60 IN'});
cutCtx.autoHarness();equal(cutCtx.st.harnessLength,null);                        // two different values: the user must choose
cutCtx.st.harnessLengthOff=true;cutCtx.st.raw.harnessLengths.pop();cutCtx.autoHarness();equal(cutCtx.st.harnessLength,null);   // a cleared choice is not re-applied
cutCtx.st.harnessLength={text:'48 IN'};cutCtx.resetCutListState();
equal([cutCtx.st.harnessLength,cutCtx.st.shieldTerm,cutCtx.st.cableSpec,cutCtx.st.quote.wireCost,cutCtx.st.quote.bomCost],[null,{},{},{},{}]);
if(!html.includes('id="cutAllowance" type="number" min="0" step="0.25" value="2"'))throw Error('default excess missing');
console.log('PASS: harness length applies only to conductors without their own length, is flagged, and never carries across drawings');

// With no drawn positions the face view must not draw a shell or a square block of cavities
// that looks like the real connector; it is a plain wrapping list that stays inside its box.
{
 const gridCtx={st:{raw:{},nets:[]},sourceCavityModel:isolate.sourceCavityModel,splitCavs:isolate.splitCavs};
 vm.createContext(gridCtx);
 vm.runInContext(block('const faceSides=','function renderConnectors(').replace(/^const faceSides=/,'var faceSides='),gridCtx);
 const labels=Array.from({length:37},(_,i)=>String(i+1));
 const g=gridCtx.faceData({tag:'P1',occupied:'',labels},'front');
 equal(g.kind,'assumed-grid');equal(g.pts.length,37);
 if(!g.gridBox||g.gridBox.w<=0||g.gridBox.h<=0)throw Error('list has no box');
 for(const p of g.pts)if(p.x-p.r<0||p.x+p.r>g.gridBox.w||p.y+p.r+12>g.gridBox.h+1)throw Error('cavity '+p.id+' outside list box');
 if(g.gridBox.w<=g.gridBox.h)throw Error('37 cavities should wrap into a wide list, not a square block');
 const ids=new Set(g.pts.map(p=>p.x+','+p.y));if(ids.size!==37)throw Error('overlapping cavities');
 const d2=gridCtx.faceData({tag:'P1',occupied:'',labels:['1','2']},'front');
 if(d2.gridBox.w<48)throw Error('small list box');
 if(!src.includes("${list?'':geo?"))throw Error('assumed list must not draw a connector shell outline');
 if(!src.includes('cavity positions are NOT shown'))throw Error('list must state it is not the physical layout');
 console.log('PASS: face view without drawn positions is a bounded list with no shell or square grid');
}

// Front and rear are both shown, and open cavities (plug) are separated from wired ones (contact + wire).
{
 const ctx={st:{raw:{cavityModels:[{tag:'P1',part:'X',all:['1','2','3','4'],allVerified:true,unused:['3','4'],unusedStatus:'',explicitUnused:[],conflicts:[]}]},nets:[{wire:'W1',endpoints:'P1:1,J1:1',awg:'22',color:'WHT'}]},sourceCavityModel:isolate.sourceCavityModel,splitCavs:isolate.splitCavs,safe:s=>String(s).replace(/[&<>"]/g,'')};
 vm.createContext(ctx);
 vm.runInContext(block('const faceSides=','function renderConnectors(').replace(/^const faceSides=/,'var faceSides='),ctx);
 const c={tag:'P1',part:'X',occupied:'1, 2',labels:['1','2','3','4']},net=ctx.faceNetRows('P1'),d=ctx.faceData(c,'front');
 equal(ctx.faceStatus(d,'1',net),'wired');
 equal(ctx.faceStatus(d,'3',net),'open');
 equal(ctx.faceStatus(d,'2',net),'wired'); // typed occupancy counts as wired
 // a held model never reports open
 ctx.st.raw.cavityModels[0].unusedStatus='HELD — no wire paths recognized at this connector';
 const h=ctx.faceData(c,'front');equal(ctx.faceStatus(h,'3',net),'unresolved');
 ctx.st.raw.cavityModels[0].unusedStatus='';
 // no verified inventory: open only against a user-entered occupied list, never otherwise
 const nm=ctx.faceData({tag:'Q1',part:'Y',occupied:'',labels:['1','2']},'front');
 equal(ctx.faceStatus(nm,'2',new Map()),'unresolved');
 const html=ctx.renderFaceView?'':'';
 if(!src.includes("(ui.mode==='both'?['front','rear']:[ui.mode]).map(s=>faceSvg(c,i,s,net,fam,ui))"))throw Error('front and rear views must both be rendered');
 if(!src.includes('sealing plug')||!src.includes('Crimp contact, insert wire'))throw Error('open vs wired action text missing');
 console.log('PASS: front and rear views, wired vs open (plug) status, held inventory never reports open');
}

// The family decoded from the part number picks the view: a stated layout is placed (and mirrored by face and
// contact sex), other families get only a shell icon and a list, and a missing sex or family never draws positions.
{
 const ctx={st:{raw:{},nets:[]},sourceCavityModel:isolate.sourceCavityModel,splitCavs:isolate.splitCavs};
 vm.createContext(ctx);
 vm.runInContext(block('const faceSides=','function renderConnectors(').replace(/^const faceSides=/,'var faceSides='),ctx);
 const pos=[];for(let i=0;i<9;i++)pos.push({label:String(i+1),x:(i%5)*2.77-5.54,y:i<5?-1.42:1.42,r:0.8});
 const fam={recognized:true,family:'D-subminiature',shape:'d-sub',sex:'PIN',geometry:{positions:pos,outline:[[-10,-4],[10,-4],[8,4],[-8,4]]}};
 const c={tag:'J1',part:'DE9P',occupied:'1',labels:pos.map(p=>p.label),contactType:''};
 const f=ctx.faceData(c,'front',fam),r=ctx.faceData(c,'rear',fam);
 equal(f.kind,'family');equal(f.pts.length,9);
 if(!(f.pts[0].x<200)||!(r.pts[0].x>200))throw Error('male front keeps pin 1 left; rear mirrors it');
 const sock=ctx.faceData({...c,part:'DE9S'},'front',{...fam,sex:'SOCKET'});
 if(!(sock.pts[0].x>200))throw Error('socket front is the mirror of the pin face');
 if(!f.geoShell||f.geoShell.length!==4)throw Error('shell outline missing');
 equal(ctx.faceData(c,'front',{...fam,sex:''}).kind,'assumed-grid'); // sex unknown: no positions
 equal(ctx.faceData({...c,contactType:'Pin'},'front',{...fam,sex:''}).kind,'family'); // sex from the connector entry
 equal(ctx.faceData(c,'front',{recognized:false,shape:'unknown'}).kind,'assumed-grid');
 const bad=ctx.faceData({...c,labels:['1','2']},'front',fam);
 if(!bad.unknownPositions.length)throw Error('layout cavities missing from the inventory must be reported');
 if(!src.includes("fetch('/api/connector-family?part='"))throw Error('part number must drive the family lookup');
 console.log('PASS: family from part number picks shell and placed layout, mirrors by face and sex, never draws without sex or family');
}

// Stored insert layouts are labelled with the view they were drawn from; the front and rear of a pin or socket connector
// are derived from that view, and a circular layout gets a round shell. Documents link only over https.
{
 const ctx={st:{raw:{},nets:[]},sourceCavityModel:isolate.sourceCavityModel,splitCavs:isolate.splitCavs};
 vm.createContext(ctx);
 vm.runInContext(block('const faceSides=','function renderConnectors(').replace(/^const faceSides=/,'var faceSides='),ctx);
 const mk=view=>({recognized:true,family:'X',shape:'circular',sex:'PIN',geometry:{shape:'circular',view,status:'REVIEWED',positions:[{label:'1',x:-10,y:0,r:0},{label:'2',x:10,y:0,r:0}]}});
 const c={tag:'P1',part:'X',occupied:'',labels:['1','2']};
 const x1=(fam,side,sex)=>ctx.faceData({...c},side,{...fam,sex}).pts[0].x;
 // pin-front layout: pin front as drawn, pin rear mirrored, socket front mirrored, socket rear as drawn
 const pf=mk('pin-front');
 if(!(x1(pf,'front','PIN')<200&&x1(pf,'rear','PIN')>200&&x1(pf,'front','SOCKET')>200&&x1(pf,'rear','SOCKET')<200))throw Error('pin-front view orientation wrong');
 // socket-front layout is the mirror of that
 const sf=mk('socket-front');
 if(!(x1(sf,'front','SOCKET')<200&&x1(sf,'front','PIN')>200&&x1(sf,'rear','PIN')<200))throw Error('socket-front view orientation wrong');
 // a rear view of a socket connector shows the same picture as the pin front
 const sr=mk('socket-rear');
 if(!(x1(sr,'rear','SOCKET')<200&&x1(sr,'front','PIN')<200))throw Error('socket-rear view orientation wrong');
 if(!ctx.faceData(c,'front',pf).geoRound)throw Error('circular layout needs a round shell');
 if(!src.includes("u.startsWith('https://')")||!src.includes('Insert layout standard:'))throw Error('standards must link over https only');
 console.log('PASS: stored insert layouts orient by view, pin/socket and side; circular shell; https-only document links');
}

// The reference-document importer reports blocked, rejected and accepted files without hiding the reason and escapes file names.
{
 const ctx={safe:s=>String(s).replace(/[&<>"]/g,m=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m]))};
 vm.createContext(ctx);
 vm.runInContext(block('function refDocHtml(','function renderConnectors('),ctx);
 const blocked=ctx.refDocHtml({ok:false,status:'MANUAL_DOWNLOAD_REQUIRED',reason:'This is a web page, not a PDF.'},'x.pdf');
 if(!blocked.includes('tag warn')||!blocked.includes('MANUAL DOWNLOAD REQUIRED')||!blocked.includes('not a PDF'))throw Error('blocked download not shown');
 const bad=ctx.refDocHtml({ok:false,status:'REJECTED',reason:'The PDF did not parse.'},'<b>y</b>.pdf');
 if(!bad.includes('tag bad')||bad.includes('<b>y</b>')||!bad.includes('&lt;b&gt;'))throw Error('rejection or escaping wrong');
 const ok=ctx.refDocHtml({ok:true,status:'ACCEPTED',reason:'Valid PDF.',pages:194,bytes:1234,sha256:'ab'},'cat.pdf');
 if(!ok.includes('tag good')||!ok.includes('194 pages')||!ok.includes('SHA-256 ab'))throw Error('accepted file not summarized');
 if(!src.includes("fetch('/api/reference-validate'"))throw Error('importer must use the validator endpoint');
 console.log('PASS: reference document importer shows blocked, rejected and accepted files and escapes names');
}

// The insert layout library is independent of connectors: every arrangement is listed, those without stored coordinates are
// disabled with a reason, and a connector can choose a stored layout when its part number does not name one.
{
 const ctx={safe:s=>String(s).replace(/[&<>"]/g,m=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])),st:{connectors:[]},TOKEN:'t',$:()=>null,renderFaceView:()=>'',renderLayoutLibrary:()=>{}};
 vm.createContext(ctx);
 vm.runInContext(block('let insertLayouts=null;','function renderLayoutLibrary(').replace('let insertLayouts=null;','var insertLayouts=null;').replace('const libGeo=new Map();let libSel=\'\',libSex=\'PIN\';','var libGeo=new Map();var libSel=\'\',libSex=\'PIN\';'),ctx);
 ctx.insertLayouts=[{id:'25-35',contacts:128,loaded:true},{id:'11-35',contacts:13,loaded:false,reason:'no table'},{id:'<b>',contacts:1,loaded:true}];
 const h=ctx.famPicker({arrangement:'25-35'},0);
 if(!h.includes('value="25-35"  selected')&&!/value="25-35"\s+selected/.test(h))throw Error('chosen layout not selected');
 if(!/value="11-35" disabled/.test(h)||!h.includes('(no coordinates stored)'))throw Error('unstored arrangement must be disabled');
 if(h.includes('<b>')||!h.includes('&lt;b&gt;'))throw Error('arrangement ids must be escaped');
 if(!h.includes('From the part number'))throw Error('part number default missing');
 if(!src.includes("fetch('/api/insert-layouts'")||!html.includes('id="layoutLibrary"'))throw Error('library panel not wired');
 console.log('PASS: insert layout library lists every arrangement, disables unstored ones, escapes ids, and lets a connector choose a layout');
}

// The family coverage audit is shown with its status, never as complete, and is escaped.
{
 const ctx={safe:s=>String(s).replace(/[&<>"]/g,m=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m]))};
 vm.createContext(ctx);
 vm.runInContext(block('function familyCoverageHtml(','function renderLayoutLibrary('),ctx);
 const h=ctx.familyCoverageHtml([{family:'<i>F</i>',standard:'S',auditStatus:'INCOMPLETE',arrangementsStored:'8',arrangementsKnown:'24',documentsRetrieved:'a',documentsManualRequired:'b',note:'n'},{family:'G',standard:'S',auditStatus:'MANUAL DOWNLOAD REQUIRED',arrangementsStored:'0',arrangementsKnown:'unknown',documentsRetrieved:'None',documentsManualRequired:'x',note:'n'}]);
 if(!h.includes('8 of 24')||!h.includes('0 of unknown')||!h.includes('no family is complete'))throw Error('coverage counts or caveat missing');
 if(h.includes('<i>F</i>')||!h.includes('&lt;i&gt;'))throw Error('family names must be escaped');
 if(/>COMPLETE</.test(h))throw Error('a family must never read complete');
 if(ctx.familyCoverageHtml([])!=='')throw Error('empty list should render nothing');
 console.log('PASS: family coverage audit shown with status and counts, escaped, never complete');
}

// Cavities the drawing says are unused are shown as NOT USED, separately from cavities that merely have no wire;
// the view can be filtered, a cavity can be pinned, and both views stay available.
{
 const els={};const ctx={st:{raw:{cavityModels:[{tag:'P1',part:'X',all:['1','2','3','4','5'],allVerified:true,unused:['3','5'],unusedStatus:'',explicitUnused:[{cavity:'4',source:'note on sheet 2'}],conflicts:[]}]},nets:[{wire:'W1',endpoints:'P1:1,J1:1',awg:'22',color:'WHT'}],connectors:[]},sourceCavityModel:isolate.sourceCavityModel,splitCavs:isolate.splitCavs,safe:s=>String(s).replace(/[&<>"]/g,m=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])),$:id=>els[id]||null,renderFaceView:()=>'<x>'};
 vm.createContext(ctx);
 vm.runInContext(block('const faceSides=','function renderConnectors(').replace(/^const faceSides=/,'var faceSides='),ctx);
 const c={id:'c1',tag:'P1',part:'X',occupied:'1, 2',labels:['1','2','3','4','5']};
 ctx.st.connectors=[c];
 const net=ctx.faceNetRows('P1'),d=ctx.faceData(c,'front');
 const S=id=>ctx.faceStatus(d,id,net);
 if(S('1')!=='wired'||S('4')!=='unused'||S('3')!=='open'||S('5')!=='open')throw Error('unused vs open vs wired wrong: '+['1','3','4','5'].map(S));
 if(d.explicitSrc.get('4')!=='note on sheet 2')throw Error('source of the unused marking lost');
 const FA=vm.runInContext('FACE_ACTION',ctx);if(!/NOT USED/.test(FA.unused)||/NOT USED/.test(FA.open))throw Error('only drawing-marked cavities may be called NOT USED outright');
 // an unused cavity that also carries a wire is a conflict, never silently unused
 const dc=ctx.faceData({...c,occupied:'1, 4'},'front');if(ctx.faceStatus(dc,'4',net)!=='conflict')throw Error('unused cavity with a wire must conflict');
 // filters
 if(!ctx.faceFilterMatch('all','wired')||!ctx.faceFilterMatch('notused','unused')||!ctx.faceFilterMatch('notused','open')||ctx.faceFilterMatch('notused','wired')||!ctx.faceFilterMatch('problem','unresolved'))throw Error('filters wrong');
 // state: mode, filter, pin / unpin
 ctx.faceSetMode(0,'rear');if(ctx.faceUiOf(c).mode!=='rear')throw Error('mode not set');
 ctx.faceSetMode(0,'bogus');if(ctx.faceUiOf(c).mode!=='rear')throw Error('invalid mode accepted');
 ctx.faceSetFilter(0,'notused');if(ctx.faceUiOf(c).filter!=='notused')throw Error('filter not set');
 ctx.faceSelect(0,'3');if(ctx.faceUiOf(c).sel!=='3')throw Error('cavity not pinned');
 ctx.faceSelect(0,'3');if(ctx.faceUiOf(c).sel!=='')throw Error('second click should unpin');
 // the svg dims non-matching cavities and rings the pinned one
 const fam={recognized:true,shape:'circular',sex:'PIN',geometry:{shape:'circular',view:'pin-front',status:'R',positions:[1,2,3,4,5].map((n,k)=>({label:String(n),x:k*3-6,y:0,r:0}))}};
 const svg=ctx.faceSvg(c,0,'front',net,fam,{mode:'both',filter:'notused',sel:'3',flip:false});
 if(!/opacity:0\.2/.test(svg)||!svg.includes('stroke="#0b5cad"'))throw Error('dimming or selection ring missing');
 if(!svg.includes('onclick="faceSelect(0,'))throw Error('cavities must be clickable');
 console.log('PASS: not-used vs open vs wired cavities, filters, view modes and pinning');
}

// The arrangement register shows counts only where they were read, lists unread arrangements without values, filters by family and escapes text.
{
 const ctx={safe:s=>String(s).replace(/[&<>"]/g,m=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m]))};
 vm.createContext(ctx);
 vm.runInContext(block('function arrangementRegisterHtml(','function renderLayoutLibrary('),ctx);
 const rows=[{family:'F1',id:'12-5',contacts:3,contactSize:'16',serviceRating:'A',sourcePage:'5',status:'EXTRACTED FROM CATALOG TEXT — UNREVIEWED'},{family:'F1',id:'<b>',contacts:0,sourcePage:'6',status:'SEEN — CELLS NOT READ',note:'no clean count'},{family:'F2',id:'8-2',contacts:0,sourcePage:'1',status:'ID ONLY — UNREVIEWED'}];
 const all=ctx.arrangementRegisterHtml(rows,'');
 if(!all.includes('3 arrangements named')||!all.includes('1 with a contact count')||!all.includes('no coordinates'))throw Error('register summary wrong');
 if(all.includes('<b>')||!all.includes('&lt;b&gt;'))throw Error('register ids must be escaped');
 if(!/<td>—<\/td><td>—<\/td><td>—<\/td>/.test(all))throw Error('unread arrangements must show no values');
 const f2=ctx.arrangementRegisterHtml(rows,'F2');if(f2.includes('12-5')||!f2.includes('8-2'))throw Error('family filter wrong');
 if(ctx.arrangementRegisterHtml([],'')!=='')throw Error('empty register should render nothing');
 if(!src.includes("fetch('/api/arrangement-register'"))throw Error('register not loaded from the endpoint');
 console.log('PASS: arrangement register shows only read values, lists unread arrangements bare, filters by family, escapes');
}

// The catalog's stated contact count is checked against the cavity inventory, entered capacity and wiring.
{
 const ctx={safe:s=>String(s).replace(/[&<>"]/g,m=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m]))};
 vm.createContext(ctx);
 vm.runInContext(block('function expectedSizes(','function renderFaceView('),ctx);
 const fam={expected:{id:'20-14',contacts:5,contactSize:'2x8;3x12',serviceRating:'A',source:'Amphenol <catalog>',sourcePage:'7',status:'EXTRACTED FROM CATALOG TEXT — UNREVIEWED'}};
 const ok=ctx.expectedHtml(fam,5,5,3);
 if(!ok.includes('COUNT AGREES')||!ok.includes('2 × #8, 3 × #12')||!ok.includes('page 7')||ok.includes('MISMATCH'))throw Error('agreeing connector shown wrong');
 if(ok.includes('<catalog>')||!ok.includes('&lt;catalog&gt;'))throw Error('source must be escaped');
 const inv=ctx.expectedHtml(fam,6,5,3);
 if(!inv.includes('MISMATCH')||!inv.includes('lists 6 cavities but the catalog arrangement has 5'))throw Error('inventory mismatch missed');
 const cap=ctx.expectedHtml(fam,5,8,3);
 if(!cap.includes('insert capacity is 8'))throw Error('capacity mismatch missed');
 const wired=ctx.expectedHtml(fam,5,5,7);
 if(!wired.includes('7 cavities carry wires but the arrangement has only 5'))throw Error('too many wired cavities missed');
 const none=ctx.expectedHtml(fam,0,0,0);
 if(!none.includes('NOT COMPARED')||none.includes('COUNT AGREES'))throw Error('nothing to compare must not read as agreement');
 if(ctx.expectedHtml({},5,5,5)!==''||ctx.expectedHtml(null,5,5,5)!=='')throw Error('no expectation, no line');
 if(ctx.expectedSizes('16')!=='#16')throw Error('single size format');
 if(!src.includes('${expectedHtml(fam,d.all.length,c.capacity,cnt.wired)}'))throw Error('connector card does not show the expectation');
 console.log('PASS: catalog contact count compared with inventory, capacity and wiring; mismatches shown, nothing compared is not agreement');
}

// A drawing may state only how many contacts are USED. That count is compared with the wired cavities, and a user can also mark cavities not used.
{
 const alerts=[];const ctx={st:{raw:{},nets:[]},sourceCavityModel:isolate.sourceCavityModel,splitCavs:isolate.splitCavs,safe:s=>String(s).replace(/[&<>"]/g,m=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])),$:()=>null,alert:m=>alerts.push(m),renderFaceView:()=>''};
 vm.createContext(ctx);
 vm.runInContext(block('const faceSides=','function renderFaceView(').replace(/^const faceSides=/,'var faceSides='),ctx);
 const mk=(over)=>({id:'c1',tag:'P1',part:'X',occupied:'1, 2, 3',labels:['1','2','3','4','5','6'],...over});
 const S=(c,id)=>{const d=ctx.faceData(c,'front');return ctx.faceStatus(d,id,ctx.faceNetRows('P1'))};
 // no count stated: cavities with no wire stay "open" (derived), as before
 let c=mk({});ctx.st.connectors=[c];
 if(S(c,'4')!=='open')throw Error('with a typed occupied list the other inventory cavities are open (derived): '+S(c,'4'));
 c=mk({usedCount:3});let d=ctx.faceData(c,'front');
 if(d.usedState!=='agree'||d.usedStated!==3||d.usedSource!=='entered by you')throw Error('matching count not recognized');
 c=mk({usedCount:5});d=ctx.faceData(c,'front');
 if(d.usedState!=='held'||d.open.size!==0)throw Error('a stated count above the wired count must hold the unused decision');
 c=mk({usedCount:2});d=ctx.faceData(c,'front');
 if(d.usedState!=='over')throw Error('a stated count below the wired count is a conflict');
 // the drawing's own count (from the model) wins over a typed one
 ctx.st.raw={cavityModels:[{tag:'P1',part:'X',all:['1','2','3','4','5','6'],allVerified:true,unused:['4','5','6'],unusedStatus:'CORROBORATED',usedStated:3,usedSource:'statement on sheet 1',explicitUnused:[],conflicts:[],wired:['1','2','3']}]};
 c=mk({usedCount:9});d=ctx.faceData(c,'front');
 if(d.usedStated!==3||d.usedSource!=='statement on sheet 1'||d.usedState!=='agree')throw Error('drawing count must win');
 if(S(c,'4')!=='open'||!d.open.has('5'))throw Error('corroborated count should leave the unmatched cavities open');
 // marking
 c=mk({manualUnused:['6']});ctx.st.connectors=[c];
 if(S(c,'6')!=='unused'||ctx.faceData(c,'front').explicitSrc.get('6')!=='marked by you')throw Error('manual mark not used');
 c=mk({manualUnused:['2']});if(S(c,'2')!=='conflict')throw Error('a wired cavity marked unused must conflict');
 c=mk({});ctx.st.connectors=[c];
 ctx.faceToggleUnused(0,'2');if(!alerts.length||(c.manualUnused||[]).length)throw Error('must refuse to mark a wired cavity');
 ctx.faceToggleUnused(0,'5');if(JSON.stringify(c.manualUnused)!=='["5"]')throw Error('mark not stored');
 ctx.faceToggleUnused(0,'5');if(c.manualUnused.length!==0)throw Error('mark not cleared');
 ctx.setUsedCount(0,'4.4');if(c.usedCount!==4)throw Error('count not stored as a whole number');
 ctx.setUsedCount(0,'-3');if(c.usedCount!=='')throw Error('negative or empty count must clear');
 // the card text
 ctx.st.raw={};c=mk({usedCount:5});d=ctx.faceData(c,'front');
 let h=ctx.usedCountHtml(c,0,d);if(!h.includes('HELD')||!h.includes('2 used cavities have no recognized wire'))throw Error('held text missing');
 c=mk({usedCount:3});h=ctx.usedCountHtml(c,0,ctx.faceData(c,'front'));if(!h.includes('COUNT MATCHES WIRING'))throw Error('match text missing');
 c=mk({});h=ctx.usedCountHtml(c,0,ctx.faceData(c,'front'));if(!h.includes('No used-contact count is stated'))throw Error('not-stated text missing');
 console.log('PASS: stated used-contact count agrees, holds or conflicts with the wiring; manual not-used marks conflict with wires');
}

// The brand mark is an inline SVG (no external file) and the tab icon is an inline data URI, both allowed by the page's CSP.
{
 if(!html.includes('class="brandmark"')||!html.includes('aria-label="Trivex"'))throw Error('sidebar mark missing or unlabeled');
 if(!/<link rel="icon" href="data:image\/svg\+xml,/.test(html))throw Error('tab icon missing');
 if(/<img[^>]+(?:src|href)=["']https?:/i.test(html))throw Error('the page must not load an external image');
 console.log('PASS: logo mark and tab icon are inline, labeled, and load nothing external');
}

// Each connector states plainly which pins are used and which are not, with the reason, and says so when it cannot tell.
{
 const ctx={st:{raw:{},nets:[{wire:'W1',endpoints:'P1:1,J2:A',awg:'22',color:'WHT',signal:'PWR'}]},sourceCavityModel:isolate.sourceCavityModel,splitCavs:isolate.splitCavs,sortCav:isolate.sortCav,connFamily:()=>null,safe:s=>String(s).replace(/[&<>"]/g,m=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m]))};
 vm.createContext(ctx);
 vm.runInContext(block('const faceSides=','function renderFaceView(').replace(/^const faceSides=/,'var faceSides='),ctx);
 const eq=(a,b,m)=>{if(a!==b)throw Error(m+': '+JSON.stringify(a)+' != '+JSON.stringify(b))};
 eq(ctx.compressRanges(['1','2','3','5','7','8','9','10','A','b']),'1–3, 5, 7–10, A, b','range compression');
 eq(ctx.compressRanges(['4','3']),'3, 4','two neighbours stay listed');
 eq(ctx.compressRanges([]),'','empty');
 const lab=n=>Array.from({length:n},(_,i)=>String(i+1));
 // wired 1,2 typed plus 1 from the net; inventory of 6
 let c={id:'a',tag:'P1',part:'X',occupied:'1, 2',labels:lab(6),capacity:6};
 let h=ctx.pinUsageHtml(c,0);
 if(!h.includes('USED 2')||!h.includes('NOT USED 4')||!h.includes('Used (2):</b> 1, 2')||!h.includes('Not used (4):</b> 3–6'))throw Error('summary wrong: '+h.slice(0,400));
 if(!h.includes('W1')||!h.includes('PWR')||!h.includes('J2:A'))throw Error('the used pin must say what it goes to');
 if(!h.includes('4 with no wire found'))throw Error('reason for not used missing');
 // nothing wired at all
 c={id:'b',tag:'P9',part:'X',occupied:'',labels:lab(6),capacity:6};
 h=ctx.pinUsageHtml(c,0);
 if(!h.includes('No used pins identified')||!h.includes('USED 0'))throw Error('must say plainly that nothing was identified');
 // no inventory: cannot list unused
 c={id:'c',tag:'P1',part:'',occupied:'1, 2',labels:[],capacity:0};
 h=ctx.pinUsageHtml(c,0);
 if(!h.includes('NOT USED unknown')||!h.includes('cannot be listed'))throw Error('unknown inventory must not read as zero unused');
 // marked by the user
 c={id:'d',tag:'P1',part:'X',occupied:'1',labels:lab(6),capacity:6,manualUnused:['6']};
 h=ctx.pinUsageHtml(c,0);
 if(!h.includes('marked not used (by you)'))throw Error('manual mark reason missing');
 // conflict
 c={id:'e',tag:'P1',part:'X',occupied:'1',labels:lab(6),capacity:6,manualUnused:['1']};
 h=ctx.pinUsageHtml(c,0);
 if(!h.includes('Conflict (1)')||!h.includes('both wired and marked not used'))throw Error('conflict missing');
 if(!src.includes('${pinUsageHtml(c,i)}')||!src.includes("+pinUsageHtml(c,i)+famPicker(c,i)+'</div>'"))throw Error('usage block missing from the view, with or without a layout');
 console.log('PASS: pin usage summary names used pins with their wires, not-used pins with the reason, and says when it cannot tell');
}

// A connector that only appears as the far end of wires still gets a card, so its used pins are shown.
{
 if(!src.includes('appears only as a wire endpoint')||!src.includes("for(const m of d.cavityModels||[])"))throw Error('endpoint-only connectors must get a card');
 if(!src.includes("!(m.wired||[]).length)continue"))throw Error('a connector with no wired cavity must not get a card');
 console.log('PASS: endpoint-only connectors get a card when they have wired pins');
}

// The Connectors tab opens with this drawing's own connectors; the reference library follows them, collapsed.
{
 const tab=html.slice(html.indexOf('<section class="tab" id="tab-connectors">'),html.indexOf('<section class="tab" id="tab-nets">'));
 const first=tab.indexOf('id="connectorCards"'),lib=tab.indexOf('id="layoutLibPanel"'),imp=tab.indexOf('id="refDocPanel"');
 if(first<0||lib<0||imp<0||!(first<lib)||!(first<imp))throw Error('the reference library must come after the connectors');
 if(!/<details class="panel" id="refLibraryDetails">/.test(tab)||/<details class="panel" id="refLibraryDetails" open/.test(tab))throw Error('the reference library must be collapsed');
 if(!tab.includes('Connectors on this drawing'))throw Error('heading');
 // on a card the layout comes before the entry fields
 const card=src.slice(src.indexOf('function renderConnectors('));
 if(!(card.indexOf('id="face-view-${i}"')>0&&card.indexOf('id="face-view-${i}"')<card.indexOf('<div class="split">')))throw Error('layout must precede the entry fields on a card');
 // inside the view: the faces come before the identification and checks, and the pin usage sits between
 const view=src.slice(src.indexOf('function renderFaceView('),src.indexOf('function renderConnectors('));
 const s=view.indexOf('${sides}'),u=view.indexOf('${pinUsageHtml(c,i)}'),idn=view.indexOf('Connector identification and checks');
 if(!(s>0&&s<u&&u<idn))throw Error('order inside the view must be faces, pin usage, identification');
 console.log('PASS: Connectors tab leads with this drawing\'s connectors and their layout; reference library is below and collapsed');
}

// Two wired cavities that are not on the insert are called out, and the rest are offered as provisional, not confirmed.
{
 const ctx={st:{raw:{cavityModels:[{tag:'P1',part:'X',all:['1','2','3','4','5','6'],allVerified:true,unused:[],unusedProvisional:['4','5','6'],unusedStatus:'HELD — resolve source and inventory conflicts',explicitUnused:[],conflicts:['Cavity 9 is wired but does not exist in the sourced cavity inventory.'],wired:['1','2','3','9']}]},nets:[]},sourceCavityModel:isolate.sourceCavityModel,splitCavs:isolate.splitCavs,sortCav:isolate.sortCav,connFamily:()=>null,safe:s=>String(s).replace(/[&<>"]/g,m=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m]))};
 vm.createContext(ctx);
 vm.runInContext(block('const faceSides=','function renderFaceView(').replace(/^const faceSides=/,'var faceSides='),ctx);
 const c={id:'a',tag:'P1',part:'X',occupied:'1, 2, 3, 9',labels:['1','2','3','4','5','6'],capacity:6};
 const d=ctx.faceData(c,'front'),net=ctx.faceNetRows('P1'),S=id=>ctx.faceStatus(d,id,net);
 if(S('9')!=='offinsert')throw Error('a wired cavity that is not on the insert must be flagged: '+S('9'));
 if(S('4')!=='provisional'||S('1')!=='wired')throw Error('provisional / wired wrong');
 if(!ctx.faceFilterMatch('notused','provisional')||!ctx.faceFilterMatch('problem','offinsert'))throw Error('filters must include the new statuses');
 const h=ctx.pinUsageHtml(c,0);
 if(!h.includes('NOT ON INSERT 1')||!h.includes('Not on this insert (1):</b> 9')||!h.includes('has only 6 cavities'))throw Error('off-insert callout missing: '+h.slice(0,300));
 if(!h.includes('Probably not used, provisional (3):</b> 4–6')||!h.includes('Not confirmed because the sources conflict'))throw Error('provisional list missing');
 if(!h.includes('1 off-insert wire is real, each needs one of these cavities, so 2 to 3 of them are unused'))throw Error('range for the unused count missing: '+h);
 if(h.includes('every cavity carries a wire'))throw Error('must not claim nothing is unused');
 if(!/NOT USED \(provisional\) 3/.test(h))throw Error('the count tag must say provisional');
 if(!/Used \(4\):<\/b> 1–3, 9/.test(h))throw Error('the used list still includes the off-insert pin so the user can see it');
 console.log('PASS: pins wired off the insert are called out; remaining cavities are offered as provisional, never confirmed');
}

// A purchased cable is cut once: its BOM Ref. Des. links it to the far ends it serves, its conductors are not cut or priced
// per foot, and the cable itself is priced as a BOM part. A far end that no cable line names stays loose wire.
{
 const eq=(a,b,m)=>{if(JSON.stringify(a)!==JSON.stringify(b))throw Error(m+': '+JSON.stringify(a)+' != '+JSON.stringify(b))};
 eq(Array.from(cutCtx.expandRefs('ETH0, 1, PORT1')),['ETH0','ETH1','PORT1'],'numeric tokens inherit the stem');
 eq(Array.from(cutCtx.expandRefs('USB DN 1, 2, 3, 4')),['USB DN 1','USB DN 2','USB DN 3','USB DN 4'],'USB tokens');
 eq(Array.from(cutCtx.expandRefs('COM1, 2, MISC')),['COM1','COM2','MISC'],'a lone word after a one-word token stays');
 eq(Array.from(cutCtx.expandRefs('AUDIO IN, OUT')),['AUDIO IN','AUDIO OUT'],'a lone word after a two-word token replaces the last word');
 const mkNet=(id,far,pin,farPin)=>({id,endpoints:'P1:'+pin+', '+far+':'+farPin,color:'',awg:'',length:'',signal:'S'+pin,wire:''});
 const nets=[mkNet('N1','USB_DN1',54,1),mkNet('N2','USB_DN1',6,2),mkNet('N3','USB_DN2',55,1),mkNet('N4','USB_DN2',7,2),mkNet('N5','ETH PORT1',52,1),mkNet('N6','LOOSE',9,1)];
 const bom=[{item:'1',part:'D38999/26FG35SA',description:'38999 Connector SKT 79 POS',reference:'P1',qty:1},{item:'2',part:'M85049/38-21N',description:'Backshell strain relief',reference:'P1',qty:1},
  {item:'7',part:'CSMUAX-3M',description:'Premium USB Cable Type A Male/Female Extension Cable, 3.0m',reference:'USB DN 1, 2',qty:2},{item:'5',part:'TRD695ABL-10',description:'Premium Cat6a Cable, RJ45/ RJ45',reference:'ETH0, 1, PORT1',qty:3}];
 const A=cutCtx.bomCableAssemblies(bom,nets);
 eq(A.assemblies.map(a=>a.farEnd+':'+a.item+':'+a.conductors),['ETH PORT1:5:1','USB_DN1:7:2','USB_DN2:7:2'],'assemblies');
 if(!A.problems.some(p=>p.includes('BOM item 5')&&p.includes('lists quantity 3 but 1 far end')))throw Error('a quantity that disagrees with the matched far ends must be reported: '+A.problems);
 if(A.assemblies.some(a=>a.farEnd==='LOOSE'))throw Error('an unnamed far end must stay loose wire');
 const cut=cutCtx.buildCutList(nets,{defUnit:'IN',allowance:2,harness:{inches:60,text:'5.0 FT'},assemblies:A.assemblies});
 eq(cut.rows.length,4,'three cables and one loose wire');
 const cab=cut.rows.filter(r=>r.purchased);
 if(cab.length!==3||cab.some(r=>r.cut!==62||r.status!=='HARNESS LENGTH'))throw Error('each cable is one cut at the stated length plus excess');
 if(!cab.find(r=>r.part==='CSMUAX-3M'&&r.conductors===2))throw Error('conductor count missing');
 eq(cut.assemblyItems.sort(),['5','7','7'].sort(),'items used');
 // the quote: no $/ft for purchased cables, the cable BOM lines are priced as parts, the loose wire still needs a cost
 const q=cutCtx.computeQuote({quote:{},qty:1,cut,bom:bom.map(b=>({...b,qty:b.qty})),laborUnitHours:1});
 if(q.wireLines.some(l=>l.label.includes('purchased cable')))throw Error('purchased cables must not need a $/ft cost');
 if(q.wireLines.length!==1)throw Error('the loose wire group is still priced per foot');
 if(!q.bomLines.some(l=>l.part==='CSMUAX-3M')||!q.bomLines.some(l=>l.part==='TRD695ABL-10'))throw Error('purchased cables are priced as BOM parts');
 // a far end named by two cable lines is not linked
 const dup=bom.concat([{item:'9',part:'OTHER',description:'Another Cable',reference:'USB DN 1',qty:1}]);
 const D=cutCtx.bomCableAssemblies(dup,nets);
 if(D.assemblies.some(a=>a.farEnd==='USB_DN1')||!D.problems.some(p=>p.includes('USB_DN1')&&p.includes('2 cable lines')))throw Error('an ambiguous far end must stay loose and be reported');
 // no cable lines: nothing changes
 eq(cutCtx.bomCableAssemblies(bom.slice(0,2),nets).assemblies.length,0,'no cable lines, no assemblies');
 console.log('PASS: purchased cables are linked by BOM Ref. Des., cut once, priced as parts; ambiguous or unnamed far ends stay loose wire');
}

// Unlabelled dimensions are candidates only: a single labelled length is still selected automatically, dimensions alone never are.
{
 cutCtx.st={raw:{harnessLengths:[{page:3,label:'CUT',text:'5.0 FT',tolerance:'±2'},{page:1,label:'DIMENSION',text:'36.00',tolerance:'±1.0'},{page:1,label:'DIMENSION',text:'24.00',tolerance:'±1.0'}]},nets:[],bom:[]};
 cutCtx.autoHarness();
 if(!cutCtx.st.harnessLength||cutCtx.st.harnessLength.text!=='5.0 FT')throw Error('the single labelled length must still be selected automatically');
 cutCtx.st={raw:{harnessLengths:[{page:1,label:'DIMENSION',text:'36.00',tolerance:'±1.0'}]},nets:[],bom:[]};
 cutCtx.autoHarness();
 if(cutCtx.st.harnessLength)throw Error('a dimension alone must never be selected automatically');
 cutCtx.st={raw:{harnessLengths:[{page:1,label:'HARNESS LENGTH',text:'48 IN'},{page:2,label:'CUT',text:'5 FT'}]},nets:[],bom:[]};
 cutCtx.autoHarness();
 if(cutCtx.st.harnessLength)throw Error('two different labelled lengths must still be left for the user');
 console.log('PASS: unlabelled dimensions are candidates only; a single labelled length is still auto-selected');
}

// Density: checking tools are collapsed so the main work stays visible; nothing that guards against guessing is dropped.
{
 const intake=html.slice(html.indexOf('<section class="tab on" id="tab-intake">'),html.indexOf('<section class="tab" id="tab-evidence">'));
 const d=intake.indexOf('<details class="techdetails">');
 if(d<0||/<details class="techdetails" open/.test(intake))throw Error('the intake technical checks must be one collapsed section');
 for(const h of ['Interpretation status','Source issues','Per-sheet extraction coverage','Recognition evidence'])if(!(intake.indexOf(h)>d))throw Error(h+' must sit inside the collapsed technical checks');
 if(!(intake.indexOf('Where this job stands')>=0&&intake.indexOf('Where this job stands')<d))throw Error('the job status stays visible above the technical checks');
 const ev=html.slice(html.indexOf('id="tab-evidence"'),html.indexOf('id="tab-notes"'));
 const e=ev.indexOf('<details class="techdetails">');
 if(e<0||!(ev.indexOf('Page review: rendered sheet')<e)||!(ev.indexOf('Embedded PDF pictures')>e))throw Error('page review stays visible; the secondary evidence tools are collapsed');
 // a connector card: the layout and used/unused pins stay visible; entry fields collapse once the part is known and the card is ready
 const card=src.slice(src.indexOf('function renderConnectors('));
 if(!card.includes("<details class=\"conn-edit\" ${c.part&&ok?'':'open'}>"))throw Error('entry fields collapse only when the part number is known and the card is ready');
 const view=src.slice(src.indexOf('function renderFaceView('),src.indexOf('function renderConnectors('));
 if(/<details open><summary><b>Connector identification/.test(view))throw Error('identification details should start collapsed');
 if(!view.includes('never assumed unused'))throw Error('the never-assume-unused statement must remain');
 if(!view.includes('${usedCountHtml(c,i,d)}')||view.indexOf('${usedCountHtml(c,i,d)}')>view.indexOf('Connector identification'))throw Error('the stated used-contact count stays visible, outside the collapsed checks');
 console.log('PASS: checking tools are collapsed; layout, pin usage and the used-contact count stay visible');
}

// Shared cleanup: long introductions fold, duplicate panel titles are hidden, secondary tools collapse, nothing is dropped from the markup.
{
 if(!/p\.textContent\.length<150/.test(html)||!html.includes("d.className='why'"))throw Error('long introductions must fold into an About this section line');
 if(!html.includes('#tab-cutlist>.panel:first-child>h2:first-child'))throw Error('panel titles that repeat the page title are hidden');
 const lab=html.slice(html.indexOf('id="tab-labor"'),html.indexOf('</section>',html.indexOf('id="tab-labor"')));
 if(!/<details class="techdetails"><summary><b>Estimate from drawing features/.test(lab)||lab.indexOf('Start with a total-hours guess')>lab.indexOf('Estimate from drawing features'))throw Error('labor: the quick guess stays open; feature estimating collapses');
 const sec=html.slice(html.indexOf('id="tab-security"'),html.indexOf('</section>',html.indexOf('id="tab-security"')));
 if(!/<details class="techdetails"><summary><b>Lookup release queue and evidence ledger/.test(sec)||sec.indexOf('Data handling boundary')>sec.indexOf('Lookup release queue'))throw Error('security: the handling boundary stays visible; queue and ledger collapse');
 const con=html.slice(html.indexOf('id="tab-connectors"'),html.indexOf('id="tab-nets"'));
 if(!/<details class="techdetails"><summary><b>Cavity import and candidate evidence/.test(con)||con.indexOf('id="connectorCards"')>con.indexOf('Cavity import and candidate evidence'))throw Error('connectors: cards first, imports collapsed after');
 console.log('PASS: shared cleanup folds introductions, hides repeated titles and collapses secondary tools without removing them');
}

// Second pass: the quote's price inputs are one card, the net diagram collapses, tooling cards keep the source but fold it.
{
 const q=html.slice(html.indexOf('id="tab-quote"'),html.indexOf('</section>',html.indexOf('id="tab-quote"')));
 if((q.match(/<div class="panel">/g)||[]).length!==2)throw Error('quote: the totals card and one price-inputs card');
 for(const h of ['Rates','Wire from the cut list','Parts from the BOM','Tooling'])if(!q.includes('>'+h+'</h4>'))throw Error('quote: missing price section '+h);
 if(!q.includes('id="quoteQty"')||!q.includes('id="quoteWireRows"')||!q.includes('id="quoteBomRows"')||!q.includes('id="quoteTooling"'))throw Error('quote: inputs and rows must remain');
 const n=html.slice(html.indexOf('id="tab-nets"'),html.indexOf('</section>',html.indexOf('id="tab-nets"')));
 if(!/<details class="techdetails"><summary><b>Logical reconstruction/.test(n)||n.indexOf('id="netRows"')>n.indexOf('Logical reconstruction'))throw Error('nets: the table first, the diagram collapsed after it');
 if(!src.includes('<details class=\\"tool-src\\"><summary>Source</summary>')&&!src.includes('<details class="tool-src"><summary>Source</summary>'))throw Error('tooling: the source stays, behind a Source line');
 console.log('PASS: quote price inputs are one card, net diagram collapses, tooling cards fold their source');
}

// Review fixes: no duplicate severity tag, a two-column quote total never scrolls sideways, BOM and net ids stay readable.
{
 if(!html.includes("String(i.Severity||'').toUpperCase()===String(CAT_LABEL[c]||'').toUpperCase()"))throw Error('alert cards must not repeat the same word as category and severity');
 if(!html.includes('#quoteTotals table{min-width:0}'))throw Error('the quote totals table must fit its card');
 if(!html.includes('#bomRows td:nth-child(3) input{min-width:220px}'))throw Error('BOM description field must stay readable');
 console.log('PASS: alert tags, quote totals and BOM/net fields fixed from the page review');
}

// Capture checks: every status is shown with its readings, disagreement opens the list, and everything is escaped.
{
 const list=[
  {id:'CONDUCTOR_COUNT',title:'Number of conductors',status:'DISAGREE',message:'The readings of the conductor count disagree.',methods:[{name:'Wire rows read',value:'95'},{name:'Cable callouts × conductors',value:'96',detail:'48 cables called out'}]},
  {id:'UNUSED_CAVITIES',title:'Empty cavities',connector:'<b>X</b>',status:'AGREE',message:'2 independent readings agree.',methods:[{name:'Insert − wired',value:'32'}]},
  {id:'A',title:'One source',status:'SINGLE SOURCE',message:'m',methods:[]},
 ];
 const esc={'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'};
 const chkCtx={safe:s=>String(s??'').replace(/[&<>"']/g,c=>esc[c])};
 vm.createContext(chkCtx);vm.runInContext(block('function captureChecksHtml(','function renderConnectors('),chkCtx);
 const h=chkCtx.captureChecksHtml(list);
 if(!h.includes('<details class="capture-checks" open>'))throw Error('a disagreement must open the capture checks');
 if(!h.includes('1 agree · 1 disagree · 1 single source'))throw Error('counts');
 if(!h.includes('48 cables called out')||!h.includes('<b>95</b> Wire rows read'))throw Error('every reading is listed with its value');
 if(h.includes('<b>X</b>')||!h.includes('&lt;b&gt;X&lt;/b&gt;'))throw Error('connector text must be escaped');
 if(chkCtx.captureChecksHtml([])!==''||chkCtx.captureChecksHtml(undefined)!=='')throw Error('nothing to show, nothing drawn');
 if(chkCtx.captureChecksHtml([{...list[1]}]).includes(' open>'))throw Error('agreeing checks stay collapsed');
 console.log('PASS: capture checks list every reading, open on disagreement, stay collapsed when everything agrees, and escape text');
}
