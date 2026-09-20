import {getJSON,failure} from './common.js';
try {
 const book = await getJSON('/api/v1/books/' + document.body.dataset.id);
 document.title = book.title + ' · The Source';
 document.querySelector('#title').textContent = book.title;
 document.querySelector('#author').textContent = book.authors;
 if (!book.available) {
  document.querySelector('iframe').hidden = true;
  document.querySelector('#status').textContent = 'This text is not installed in this library.';
 }
} catch(error) { failure(error); }
