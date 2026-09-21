import {getJSON,node,link,failure} from './common.js';
const params = new URLSearchParams(location.search);
if (!params.has('limit')) params.set('limit', '30');
const form = document.querySelector('form');
form.elements.q.value = params.get('q') || '';
form.elements.available.value = params.get('available') || '';
form.addEventListener('formdata', event => {
 if (!form.elements.available.value) event.formData.delete('available');
});
document.querySelector('#api-link').href = '/api/v1/books?' + params;
const language = params.get('language') || 'en';
if (![...form.elements.language.options].some(option => option.value === language)) {
 form.elements.language.add(new Option(language,language));
}
form.elements.language.value = language;
getJSON('/api/v1/languages').then(data => {
 for (const code of data.languages) {
  if (![...form.elements.language.options].some(option => option.value === code)) {
   form.elements.language.add(new Option(code,code));
  }
 }
}).catch(() => {});
try {
 const data = await getJSON('/api/v1/books?' + params);
 document.querySelector('#status').textContent = data.total.toLocaleString() + ' matching books' + (data.books.length ? '' : ' · Try broadening your search.');
 const results = document.querySelector('#results');
 if (data.books.length) {
  const table = node('table', undefined, 'book-table');
  const thead = node('thead'); const head = node('tr');
  for (const h of ['#', 'Title', 'Author', 'Subjects', 'Lang']) head.append(node('th', h));
  thead.append(head); table.append(thead);
  const tbody = node('tbody');
  for (const book of data.books) {
   const row = node('tr');
   row.append(node('td', String(book.id), 'col-id'));
   const title = node('td', undefined, 'col-title'); title.append(link(book.title, '/books/' + book.id)); row.append(title);
   row.append(node('td', book.authors || '—', 'col-author'));
   row.append(node('td', book.subjects.slice(0,3).join(' · ') || '—', 'col-subjects'));
   row.append(node('td', book.languages.join(', '), 'col-lang'));
   tbody.append(row);
  }
  table.append(tbody); results.append(table);
 }
 const pagination = document.querySelector('#pagination');
 const offset = Number(params.get('cursor') || 0);
 const limit = Number(params.get('limit') || 30);
 if (data.books.length) document.querySelector('#status').append(document.createTextNode(' · Showing ' + (offset + 1) + '–' + (offset + data.books.length)));
 function pageLink(text,cursor,cls) {
  const next = new URLSearchParams(params); next.set('cursor',String(cursor));
  const a = link(text,'/books?' + next); a.className = cls; return a;
 }
 if (offset > 0) pagination.append(pageLink('← Previous',Math.max(0,offset-limit),'prev'));
 if (data.next_cursor) pagination.append(pageLink('Next →',data.next_cursor,'next'));
} catch(error) { failure(error); }
