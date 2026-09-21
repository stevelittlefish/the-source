import {getJSON,node,link,failure} from './common.js';
const params = new URLSearchParams(location.search);
if (!params.has('limit')) params.set('limit', '30');
const form = document.querySelector('form');
form.elements.q.value = params.get('q') || '';
document.querySelector('#api-link').href = '/api/v1/lyrics?' + params;
const tag = params.get('tag') || '';
const language = params.get('language') || 'all';
// Show the current filters immediately, then fill the rest of the options in.
if (tag && ![...form.elements.tag.options].some(option => option.value === tag)) form.elements.tag.add(new Option(tag, tag));
form.elements.tag.value = tag;
if (![...form.elements.language.options].some(option => option.value === language)) form.elements.language.add(new Option(language, language));
form.elements.language.value = language;
form.addEventListener('formdata', event => {
 if (!form.elements.tag.value) event.formData.delete('tag');
 if (form.elements.language.value === 'all') event.formData.delete('language');
});
function fill(select, path, key, current) {
 return getJSON(path).then(data => {
  for (const value of data[key]) if (![...select.options].some(option => option.value === value)) select.add(new Option(value, value));
  select.value = current;
 }).catch(() => {});
}
fill(form.elements.tag, '/api/v1/lyrics/tags', 'tags', tag);
fill(form.elements.language, '/api/v1/lyrics/languages', 'languages', language);
try {
 const data = await getJSON('/api/v1/lyrics?' + params);
 document.querySelector('#status').textContent = data.total.toLocaleString() + ' matching songs' + (data.songs.length ? '' : ' · Try broadening your search.');
 const results = document.querySelector('#results');
 if (data.songs.length) {
  const table = node('table', undefined, 'book-table');
  const thead = node('thead'); const head = node('tr');
  for (const h of ['#', 'Title', 'Artist', 'Genre', 'Year', 'Views']) head.append(node('th', h));
  thead.append(head); table.append(thead);
  const tbody = node('tbody');
  for (const song of data.songs) {
   const row = node('tr');
   row.append(node('td', String(song.id), 'col-id'));
   const title = node('td', undefined, 'col-title'); title.append(link(song.title, '/lyrics/' + song.id)); row.append(title);
   row.append(node('td', song.artist || '—', 'col-author'));
   row.append(node('td', song.tag || '—', 'col-lang'));
   row.append(node('td', song.year ? String(song.year) : '—', 'col-lang'));
   row.append(node('td', (song.views || 0).toLocaleString(), 'col-views'));
   tbody.append(row);
  }
  table.append(tbody); results.append(table);
 }
 const pagination = document.querySelector('#pagination');
 const offset = Number(params.get('cursor') || 0);
 const limit = Number(params.get('limit') || 30);
 if (data.songs.length) document.querySelector('#status').append(document.createTextNode(' · Showing ' + (offset + 1) + '–' + (offset + data.songs.length)));
 function pageLink(text, cursor, cls) {
  const next = new URLSearchParams(params); next.set('cursor', String(cursor));
  const a = link(text, '/lyrics/browse?' + next); a.className = cls; return a;
 }
 if (offset > 0) pagination.append(pageLink('← Previous', Math.max(0, offset - limit), 'prev'));
 if (data.next_cursor) pagination.append(pageLink('Next →', data.next_cursor, 'next'));
} catch (error) { failure(error); }
