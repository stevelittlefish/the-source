import {getJSON,failure} from './common.js';
const id = document.body.dataset.id;
const more = document.querySelector('#load-more');
const text = document.querySelector('#text');
const status = document.querySelector('#status');
let offset = 0;
const decoder = new TextDecoder('utf-8');
async function loadText() {
 more.disabled = true;
 status.textContent = 'Loading text…';
 try {
  const response = await fetch('/api/v1/books/' + id + '/text', {headers:{Range:'bytes=' + offset + '-' + (offset + 65535)}});
  if (!response.ok) throw new Error('Could not load book text. Try again or open Plain text.');
  const bytes = await response.arrayBuffer();
  const range = response.headers.get('Content-Range');
  const total = range ? Number(range.split('/')[1]) : bytes.byteLength;
  offset += bytes.byteLength;
  const finished = offset >= total || response.status === 200;
  text.append(document.createTextNode(decoder.decode(bytes,{stream:!finished})));
  status.textContent = finished ? 'End of text · ' + offset.toLocaleString() + ' bytes loaded' : offset.toLocaleString() + ' of ' + total.toLocaleString() + ' bytes loaded';
  more.hidden = finished;
 } catch(error) { failure(error); more.hidden = false; }
 finally { more.disabled = false; }
}
more.addEventListener('click',loadText);
try {
 const book = await getJSON('/api/v1/books/' + id);
 document.title = book.title + ' · The Source';
 document.querySelector('#title').textContent = book.title;
 document.querySelector('#author').textContent = book.authors;
 if (book.available) await loadText();
 else { text.hidden = true; status.textContent = 'This text is not installed in this library.'; }
} catch(error) { failure(error); }
