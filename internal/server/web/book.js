import {getJSON,node,link,failure,yearFilters} from './common.js';
try {
 const random = location.pathname === '/random';
 const params = new URLSearchParams(location.search);
 if (random) {
  yearFilters(document.querySelector('form'),params);
  const select = document.querySelector('select[name="language"]');
  const language = params.get('language') || 'en';
  if (![...select.options].some(option => option.value === language)) select.add(new Option(language,language));
  select.value = language;
  getJSON('/api/v1/languages').then(data => {
   for (const code of data.languages) if (![...select.options].some(option => option.value === code)) select.add(new Option(code,code));
  }).catch(() => {});
 }
 const book = await getJSON(random ? '/api/v1/books/random?' + params : '/api/v1/books/' + document.body.dataset.id);
 document.querySelector('#book-id').textContent = book.id;
 document.title = book.title + ' · The Source';
 document.querySelector('#title').textContent = book.title;
 document.querySelector('#author').textContent = book.authors;
 document.querySelector('#status').textContent = book.available ? 'Text installed and ready to read.' : 'Catalogue only. This text is not installed in this library.';
 const metadata = document.querySelector('#metadata');
 metadata.append(node('dt','Original publication year'),node('dd',book.original_publication_year || 'Unknown'));
 document.querySelector('#json').textContent = JSON.stringify(book,null,2);
 for (const [name,value] of [['Gutenberg ID',String(book.id)],['Type',book.type],['Languages',book.languages.join(', ')],['Issued',book.issued],['Library of Congress',book.locc.join(', ')],['Subjects',book.subjects.join(' · ')],['Bookshelves',book.bookshelves.join(' · ')]]) {
  metadata.append(node('dt',name),node('dd',value || '—'));
 }
 const actions = document.querySelector('#actions');
 if (book.available) {
  const read = link('Read this book →','/read/' + book.id);
  read.className = 'button';
  const download = link('Download text ↓','/api/v1/books/' + book.id + '/text');
  download.className = 'button button-secondary';
  download.download = 'pg' + book.id + '.txt';
  actions.append(read,download);
 }
 const links = node('span',undefined,'action-links');
 if (random) links.append(link('Permanent book page','/books/' + book.id));
 links.append(link('View API record','/api/v1/books/' + book.id));
 actions.append(links);
} catch(error) {
 failure(error);
 document.querySelector('#title').textContent = 'No book to show';
 document.querySelector('#json').textContent = error.message;
}
