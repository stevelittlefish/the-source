import {getJSON,node,failure} from './common.js';
const params = new URLSearchParams(location.search);
const form = document.querySelector('form');
const select = form.elements.chain;
form.elements.count.value = params.get('count') || '5';
form.elements.seed.value = params.get('seed') || '';
form.addEventListener('formdata', event => { if (!form.elements.seed.value) event.formData.delete('seed'); });
try {
 const list = await getJSON('/api/v1/markov');
 if (!list.chains.length) throw new Error('No Markov chains are installed on this server.');
 for (const chain of list.chains) select.add(new Option(`${chain.name} · ${chain.kind}s from ${chain.trained_on.toLocaleString()}`, chain.name));
 const wanted = params.get('chain');
 select.value = list.chains.some(chain => chain.name === wanted) ? wanted : list.chains[0].name;
 const query = new URLSearchParams({count: form.elements.count.value});
 if (form.elements.seed.value) query.set('seed', form.elements.seed.value);
 const path = '/api/v1/markov/' + encodeURIComponent(select.value) + '?' + query;
 document.querySelector('#api-link').href = path;
 const data = await getJSON(path);
 const results = document.querySelector('#results');
 for (const result of data.results) results.append(node(data.kind === 'verse' ? 'pre' : 'p', result.text, 'reader'));
 document.querySelector('#status').textContent = data.results.length + ' ' + data.kind + (data.results.length === 1 ? '' : 's') + ' from ' + data.chain + (data.seed !== undefined ? ' · seed ' + data.seed : '');
 document.querySelector('#json').textContent = JSON.stringify(data, null, 2);
} catch (error) { failure(error); }
