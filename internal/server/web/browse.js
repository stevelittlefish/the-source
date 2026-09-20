import {getJSON,node,link,failure} from './common.js';
const params = new URLSearchParams(location.search);
const form = document.querySelector('form');
form.elements.q.value = params.get('q') || '';
form.elements.available.checked = params.get('available') === 'true';
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
 for (const book of data.books) {
  const card = node('article');
  const title = node('h2'); title.append(link(book.title, '/books/' + book.id));
  card.append(node('p', '#' + book.id + ' · ' + book.languages.join(', '), 'eyebrow'), title,
   node('p', book.authors || 'Author unrecorded'), node('span', book.available ? 'Text installed' : 'Catalogue only', 'badge'));
  results.append(card);
 }
 const pagination = document.querySelector('#pagination');
 const offset = Number(params.get('cursor') || 0);
 const limit = Number(params.get('limit') || 25);
 function pageLink(text,cursor) {
  const next = new URLSearchParams(params); next.set('cursor',String(cursor));
  return link(text,'/books?' + next);
 }
 if (offset > 0) pagination.append(pageLink('← Previous',Math.max(0,offset-limit)));
 if (data.next_cursor) pagination.append(pageLink('Next →',data.next_cursor));
} catch(error) { failure(error); }
