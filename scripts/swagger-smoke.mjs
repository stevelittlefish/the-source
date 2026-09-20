// Exercise the real explorer without a browser automation dependency.
import {spawn} from 'node:child_process';
import {mkdtempSync,rmSync,existsSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import assert from 'node:assert/strict';

const base = process.argv[2] || 'http://127.0.0.1:45068';
const binary = ['/usr/bin/google-chrome','/usr/bin/chromium'].find(existsSync);
assert(binary,'Chrome or Chromium required');
const profile = mkdtempSync(join(tmpdir(),'source-swagger-'));
const chrome = spawn(binary,['--headless','--no-sandbox','--disable-gpu','--disable-dev-shm-usage','--remote-debugging-port=0','--user-data-dir='+profile,'about:blank']);
let socket;
try {
 const endpoint = await new Promise((resolve,reject) => {
  let output='';
  const timer=setTimeout(()=>reject(new Error('Chrome startup timeout')),15000);
  chrome.stderr.on('data',chunk=>{
   output+=chunk;
   const match=output.match(/DevTools listening on (ws:\/\/[^\s]+)/);
   if(match) { clearTimeout(timer); resolve(match[1]); }
  });
  chrome.on('error',reject);
 });
 socket=new WebSocket(endpoint);
 await new Promise((resolve,reject)=>{socket.onopen=resolve;socket.onerror=reject;});
 let next=0;
 const pending=new Map();
 socket.onmessage=event=>{
  const message=JSON.parse(event.data);
  const task=pending.get(message.id);
  if(task) { pending.delete(message.id); message.error ? task.reject(new Error(JSON.stringify(message.error))) : task.resolve(message.result); }
 };
 function send(method,params={},sessionId) {
  return new Promise((resolve,reject)=>{
   const id=++next;
   pending.set(id,{resolve,reject});
   socket.send(JSON.stringify({id,method,params,sessionId}));
  });
 }
 const {targetId}=await send('Target.createTarget',{url:base+'/docs'});
 const {sessionId}=await send('Target.attachToTarget',{targetId,flatten:true});
 async function evaluate(expression) {
  const response=await send('Runtime.evaluate',{expression,returnByValue:true},sessionId);
  if(response.exceptionDetails) throw new Error(JSON.stringify(response.exceptionDetails));
  return response.result.value;
 }
 async function waitFor(expression) {
  for(let i=0;i<100;i++) {
   if(await evaluate(expression)) return;
   await new Promise(resolve=>setTimeout(resolve,100));
  }
  throw new Error('Browser condition timed out: '+expression);
 }
 await waitFor("!!document.querySelector('#operations-default-getHealth')");
 await evaluate("document.querySelector('#operations-default-getHealth .opblock-summary-control').click()");
 await waitFor("!!document.querySelector('#operations-default-getHealth .try-out__btn')");
 await evaluate("document.querySelector('#operations-default-getHealth .try-out__btn').click()");
 await evaluate("document.querySelector('#operations-default-getHealth .execute').click()");
 await waitFor("document.querySelector('#operations-default-getHealth .live-responses-table')?.textContent.includes('ok')");
 const result = await evaluate("document.querySelector('#operations-default-getHealth .live-responses-table').textContent");
 assert(result.includes('200') && result.includes('ok'),result);
 console.log('PASS Swagger UI loaded the spec and executed GET /health');
} finally {
 socket?.close();
 const exited=new Promise(resolve=>chrome.once('exit',resolve));
 if(chrome.exitCode===null) { chrome.kill('SIGTERM'); await exited; }
 rmSync(profile,{recursive:true,force:true});
}
