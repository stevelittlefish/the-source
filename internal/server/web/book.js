import {getJSON,node,link,failure} from './common.js';
try {
 const book = await getJSON('/api/v1/books/' + document.body.dataset.id);
 document.title = book.title + ' · The Source';
 document.querySelector('#title').textContent = book.title;
 document.querySelector('#author').textContent = book.authors;
 document.querySelector('#status').textContent = book.available ? 'Text installed and ready to read.' : 'Catalogue only. This text is not installed in this library.';
 const metadata = document.querySelector('#metadata');
 for (const [name,value] of [['Languages',book.languages.join(', ')],['Issued',book.issued],['Subjects',book.subjects.join(' · ')],['Bookshelves',book.bookshelves.join(' · ')]]) {
  metadata.append(node('dt',name),node('dd',value || '—'));
 }
 const actions = document.querySelector('#actions');
 if (book.available) actions.append(link('Read this book →','/read/' + book.id),document.createTextNode(' · '));
 actions.append(link('View API record','/api/v1/books/' + book.id));
} catch(error) { failure(error); }
