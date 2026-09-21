import {getJSON,node,link,failure} from './common.js';
function fill(select, path, key, current) {
 return getJSON(path).then(data => {
  for (const value of data[key]) if (![...select.options].some(option => option.value === value)) select.add(new Option(value, value));
  select.value = current;
 }).catch(() => {});
}
try {
 const random = location.pathname === '/lyrics/random';
 const params = new URLSearchParams(location.search);
 if (random) {
  const form = document.querySelector('form');
  const tag = params.get('tag') || '';
  const language = params.get('language') || 'all';
  if (tag && ![...form.elements.tag.options].some(option => option.value === tag)) form.elements.tag.add(new Option(tag, tag));
  form.elements.tag.value = tag;
  if (![...form.elements.language.options].some(option => option.value === language)) form.elements.language.add(new Option(language, language));
  form.elements.language.value = language;
  for (const key of ['views_from', 'views_to']) form.elements[key].value = params.get(key) || '';
  form.addEventListener('formdata', event => {
   if (!form.elements.tag.value) event.formData.delete('tag');
   if (form.elements.language.value === 'all') event.formData.delete('language');
   for (const key of ['views_from', 'views_to']) if (!form.elements[key].value) event.formData.delete(key);
  });
  fill(form.elements.tag, '/api/v1/lyrics/tags', 'tags', tag);
  fill(form.elements.language, '/api/v1/lyrics/languages', 'languages', language);
 }
 const song = await getJSON(random ? '/api/v1/lyrics/random?' + params : '/api/v1/lyrics/' + document.body.dataset.id);
 document.querySelector('#song-id').textContent = song.id;
 document.title = song.title + ' · The Source';
 document.querySelector('#title').textContent = song.title;
 document.querySelector('#author').textContent = song.artist + (song.features && song.features.length ? ' (feat. ' + song.features.join(', ') + ')' : '');
 document.querySelector('#status').textContent = (song.views || 0).toLocaleString() + ' views' + (song.year ? ' · ' + song.year : '');
 document.querySelector('#json').textContent = JSON.stringify(song, null, 2);
 const metadata = document.querySelector('#metadata');
 for (const [name, value] of [['Song ID', String(song.id)], ['Artist', song.artist], ['Genre', song.tag], ['Language', song.language], ['Year', song.year ? String(song.year) : 'Unknown'], ['Views', (song.views || 0).toLocaleString()], ['Featured', (song.features || []).join(', ')]]) {
  metadata.append(node('dt', name), node('dd', value || '—'));
 }
 const actions = document.querySelector('#actions');
 const download = link('Download lyrics ↓', '/api/v1/lyrics/' + song.id + '/text');
 download.className = 'button button-secondary';
 download.download = song.id + '.txt';
 actions.append(download);
 const links = node('span', undefined, 'action-links');
 if (random) links.append(link('Permanent song page', '/lyrics/' + song.id));
 links.append(link('View API record', '/api/v1/lyrics/' + song.id));
 actions.append(links);
 const body = await fetch('/api/v1/lyrics/' + song.id + '/text');
 document.querySelector('#text').textContent = body.ok ? await body.text() : 'Lyrics unavailable.';
} catch (error) {
 failure(error);
 document.querySelector('#title').textContent = 'No song to show';
 document.querySelector('#text').textContent = '';
 document.querySelector('#json').textContent = error.message;
}
