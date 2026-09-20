// Optional test tooling: no packages or frontend build ceremony required.
import {spawnSync} from 'node:child_process';
import {existsSync,mkdtempSync,rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import assert from 'node:assert/strict';

const chrome = ['/usr/bin/chromium','/usr/bin/google-chrome','/usr/bin/chromium-browser'].find(existsSync);
assert(chrome, 'Install Chromium or Chrome to run this optional smoke test.');
const base = process.argv[2] || 'http://127.0.0.1:45068';
for (const [path,expected] of [
 ['/books?available=true', 'Pride and Prejudice'],
 ['/books?q=zzzznosuchtitlezzzz', '0 matching books'],
 ['/books/1342', 'Read this book'],
 ['/read/1342', 'LOCAL DEVELOPMENT FIXTURE'],
 ['/books/1342', '"locc":'],
 ['/random', 'Permanent book page'],
 ['/excerpts', '3 complete paragraphs'],
 ['/random?language=zz', 'No installed texts match this language.'],
]) {
 const profile = mkdtempSync(join(tmpdir(),'source-browser-'));
 try {
  const result = spawnSync(chrome,[
   '--headless','--no-sandbox','--disable-gpu','--disable-dev-shm-usage',
   '--user-data-dir='+profile,'--virtual-time-budget=5000','--dump-dom',base+path,
  ],{encoding:'utf8',timeout:20000,maxBuffer:4*1024*1024});
  assert.equal(result.status,0,result.stderr);
  assert(result.stdout.includes(expected),path+' did not render '+expected);
  console.log('PASS '+path);
 } finally { rmSync(profile,{recursive:true,force:true}); }
}
