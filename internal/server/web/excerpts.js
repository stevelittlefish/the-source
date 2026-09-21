import {getJSON,node,link,failure,yearFilters} from './common.js';
const params = new URLSearchParams(location.search);
const form = document.querySelector('form');
yearFilters(form,params);
form.elements.paragraphs.value = params.get('paragraphs') || '3';
const language = params.get('language') || 'en';
const select = form.elements.language;
if (![...select.options].some(option => option.value === language)) select.add(new Option(language,language));
select.value = language;
getJSON('/api/v1/books/languages').then(data => {
 for (const code of data.languages) if (![...select.options].some(option => option.value === code)) select.add(new Option(code,code));
}).catch(() => {});
try {
 const data = await getJSON('/api/v1/books/excerpts/random?' + params);
 document.querySelector('#source-title').textContent = data.book.title;
 document.querySelector('#source-author').textContent = data.book.authors + ' · Original publication: ' + (data.book.original_publication_year || 'unknown');
 document.querySelector('#status').textContent = data.paragraphs.length + ' complete paragraphs · ' + data.book.languages.join(', ');
 document.querySelector('#excerpt').append(...data.paragraphs.map(p => node('p',p)));
 document.querySelector('#source-links').append(link('Book details','/books/' + data.book.id),link('Read the book →','/read/' + data.book.id));
 document.querySelector('#json').textContent = JSON.stringify(data,null,2);
} catch(error) { failure(error); document.querySelector('#excerpt').hidden = true; }
