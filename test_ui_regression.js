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
if(!src.includes('tableIds=[...new Set([...d.pts.map(p=>p.id).filter(Boolean),...d.all])]'))throw Error('missing inventory cavities in details table');
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

const featureA={conductors:10,connectors:2,branches:1,sheathingFeet:0,overbraidFeet:0,floatingShields:2,terminatedShields:2,labels:4,backshells:2};
const featureB={...featureA,conductors:20,sheathingFeet:6,overbraidFeet:4,terminatedShields:6,labels:8};
const refs=[{...ref,features:featureA,featuresReviewed:true},{...ref,name:'Invented second',estimated:13,features:featureB,featuresReviewed:true}];
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
