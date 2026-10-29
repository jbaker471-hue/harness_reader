// Runs the actual console-less Windows executable using synthetic input only.
const {spawn}=require('node:child_process');
const fs=require('node:fs');
const os=require('node:os');
const path=require('node:path');
const assert=require('node:assert/strict');
const delay=ms=>new Promise(r=>setTimeout(r,ms));
(async()=>{
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'trivex-smoke-'));
 const info=path.join(dir,'session.txt');
 const child=spawn(path.resolve(process.argv[2]),[],{stdio:'ignore',env:{...process.env,TRIVEX_NO_WINDOW:'1',TRIVEX_TEST_HEARTBEAT:'1',TRIVEX_URL_FILE:info}});
 let ended=false;let spawnError;
 child.on('exit',()=>ended=true);child.on('error',e=>{spawnError=e;ended=true});
 try{
  for(let i=0;i<120&&!fs.existsSync(info)&&!ended;i++)await delay(500);
  if(spawnError)throw spawnError;
  assert(fs.existsSync(info),'Executable did not start a local server');
  const values=Object.fromEntries(fs.readFileSync(info,'utf8').trim().split(/\r?\n/).map(s=>{const i=s.indexOf('=');return [s.slice(0,i),s.slice(i+1)]}));
  const url=values.TRIVEX_URL,token=values.TRIVEX_TOKEN;
  assert(url.startsWith('http://127.0.0.1:'));
  async function api(route,body,method='POST'){
   const r=await fetch(url+route,{method,headers:{'X-Trivex-Token':token},body:body===undefined?undefined:JSON.stringify(body),signal:AbortSignal.timeout(120000)});
   assert(r.ok,route+' status '+r.status+' '+await (!r.ok?r.text():Promise.resolve('')));
   return r.json();
  }
  assert.equal((await fetch(url+'api/version')).status,403);
  await api('api/heartbeat');
  const v=await api('api/version',undefined,'GET');
  assert.equal(v.engineStatus,'ok');assert(v.version.startsWith('2.8'));
  const report=await api('api/self-test');assert(report.pass,JSON.stringify(report));
  await api('api/heartbeat');
  const fixtures=fs.readdirSync('testdata',{recursive:true}).filter(n=>n.endsWith('.pdf')).sort();
  for(const name of fixtures){
   const r=await fetch(url+'api/parse',{method:'POST',headers:{'X-Trivex-Token':token},body:fs.readFileSync(path.join('testdata',name)),signal:AbortSignal.timeout(120000)});
   const parsed=await r.json();assert(r.ok,name+' parse failed');assert(parsed.pages>0,name+' has no pages');
   if(name==='SYNTHETIC_V24_TABLE_NETLIST.pdf'){assert.equal(parsed.wires.length,4);assert.equal(parsed.boms.length,3);}
   await api('api/heartbeat');
  }
  const guide=await api('api/resolve',{part:'D38999/26WB35PN'});
  assert.equal(guide.status,'verified-layout');assert.equal(guide.labels.length,13);assert.equal(guide.labels[12],'13');assert(guide.source.includes('Glenair'));
  const unknown=await api('api/resolve',{part:'D38999/26WB99PN'});assert.equal(unknown.labels.length,0);
  const accessories=await api('api/assembly-library',undefined,'GET');
  const standardBand=accessories.find(x=>x.kind==='band'&&x.matches.includes('600-052'));
  assert(standardBand,'600-052 band reference missing from packaged executable');
  assert(standardBand.requirement.includes('600-058'),'600-052 must select banding tool 600-058');
  assert.equal(standardBand.specification,'M81306/1A');
  assert(standardBand.alternates.includes('DMC DBS-2100'),'600-052 must expose DMC DBS-2100 as a shared-specification alternate');
  const milMicroBand=accessories.find(x=>x.kind==='band'&&x.matches.includes('M85049/128-7'));
  assert(milMicroBand?.requirement.includes('600-061'),'M85049/128-7 must select micro banding tool 600-061');
  assert.equal(milMicroBand.specification,'M81306/1B');
  assert(milMicroBand.alternates.includes('DMC DBS-2200'),'M85049/128-7 must expose DMC DBS-2200 as a shared-specification alternate');
  const crimpRing=accessories.find(x=>x.kind==='crimpring'&&x.matches.includes('M85049/20'));
  assert(crimpRing?.requirement.includes('crimp-ring'),'M85049/20 crimp-ring tooling hold missing from packaged executable');
  const clamp=accessories.find(x=>x.kind==='clamp'&&x.matches.includes('M85049/38'));
  assert(clamp?.requirement.includes('flat-blade'),'M85049/38 clamp driver guidance missing from packaged executable');
  const tinel=accessories.find(x=>x.kind==='tinel'&&x.matches.includes('TXR'));
  assert(tinel?.requirement.includes('AD-5000'),'Tinel-Lock installation tool missing from packaged executable');
  const wireRefs=await api('api/wire-cable-library',undefined,'GET');
  assert(wireRefs.length>=12,'wire/cable reference library is incomplete');
  assert(wireRefs.some(x=>x.family.includes('AS22759 medium-weight')&&x.preparation.includes('strip')),'AS22759 preparation reference missing');
  assert(wireRefs.some(x=>x.family.includes('Multi-axial')&&x.test.includes('impedance')),'twinax/quadrax test requirements missing');
  const equivalences=await api('api/tool-equivalence-library',undefined,'GET');
  const decoded=await api('api/wire-decode?part=M27500-22RC2S06',undefined,'GET');
  assert.equal(decoded.conductors,2);assert(decoded.fields.some(f=>f.label==='Shield'&&f.value.includes('silver')&&f.sources.length&&f.status.includes('UNVERIFIED')),'wire decode shield/source missing from packaged executable');
  assert((await api('api/wire-decode?part=M27500-22ZZ2Q99',undefined,'GET')).unresolved.length===3,'unknown wire codes must stay unresolved');
  assert.equal((await fetch(url+'api/wire-decode?part=M27500-22RC2S06')).status,403);
  const fam=await api('api/connector-family?part=DB25P',undefined,'GET');assert(fam.shape==='d-sub'&&fam.geometry.positions.length===25&&fam.status.includes('UNVERIFIED'),'connector family layout missing from packaged executable');
  assert.equal((await api('api/connector-family?part=ZZZ-1',undefined,'GET')).recognized,false);
  assert.equal((await fetch(url+'api/connector-family?part=DB25P')).status,403);
  const f38=await api('api/connector-family?part=D38999/26FD35PN',undefined,'GET');assert(f38.layoutStandards.some(x=>x.code==='MIL-STD-1560'&&x.sources.length)&&f38.specs.length===1&&!f38.geometry,'connector documentation links or no-invented-geometry rule missing from packaged executable');
  assert(equivalences.some(x=>x.specification==='AS22520/1'&&x.primary.includes('AF8')&&x.conditions.includes('exact turret/positioner')),'AF8 powered-route conditions missing');
  assert(equivalences.some(x=>x.specification==='AS22520/23'&&x.status.includes('QUALIFICATION REVIEW')),'large-gauge qualification safeguard missing');
  const plan=await api('api/build-plan',{buildQty:2,policy:'plug-only',connectors:[{tag:'P1',part:'D38999/26FG35PA',all:['1','2'],allVerified:true,wired:[{cavity:'1',awg:'24'}]}]});
  assert.equal(plan.buildQty,2);assert.equal(plan.connectors[0].crimp[0].status,'OK');
  assert.equal(plan.connectors[0].unused.length,1);
  await api('api/closing');
  for(let i=0;i<30&&!ended;i++)await delay(500);
  assert(ended,'Closing signal failed to stop executable');
  assert.equal(child.exitCode,0,'Executable exited with an error');
  console.log('PASS standalone Windows GUI-subsystem executable: offline PDF engine, token protection, synthetic PDF API, build plan, closing shutdown');
 }finally{if(!ended)child.kill();fs.rmSync(dir,{recursive:true,force:true});}
})().catch(e=>{console.error(e);process.exitCode=1});
