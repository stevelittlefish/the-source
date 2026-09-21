import {getJSON,link,failure} from './common.js';
const params = new URLSearchParams(location.search);
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
function fill(select, path, key, current) {
 return getJSON(path).then(data => {
  for (const value of data[key]) if (![...select.options].some(option => option.value === value)) select.add(new Option(value, value));
  select.value = current;
 }).catch(() => {});
}
fill(form.elements.tag, '/api/v1/lyrics/tags', 'tags', tag);
fill(form.elements.language, '/api/v1/lyrics/languages', 'languages', language);
try {
 const data = await getJSON('/api/v1/lyrics/excerpts/random?' + params);
 document.querySelector('#source-title').textContent = data.song.title;
 document.querySelector('#source-author').textContent = data.song.artist + (data.song.year ? ' · ' + data.song.year : '') + (data.song.tag ? ' · ' + data.song.tag : '');
 document.querySelector('#status').textContent = data.lines.length + ' lines · ' + (data.song.views || 0).toLocaleString() + ' views';
 document.querySelector('#excerpt').textContent = data.lines.join('\n');
 document.querySelector('#source-links').append(link('Song details', '/lyrics/' + data.song.id));
 document.querySelector('#json').textContent = JSON.stringify(data, null, 2);
} catch (error) { failure(error); document.querySelector('#excerpt').hidden = true; }
