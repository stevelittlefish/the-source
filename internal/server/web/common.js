export async function getJSON(path) {
 const response = await fetch(path);
 const data = await response.json();
 if (!response.ok) throw new Error(data.error?.message || 'The library is temporarily unavailable.');
 return data;
}
export function node(tag, text, className) {
 const element = document.createElement(tag);
 if (text !== undefined) element.textContent = text;
 if (className) element.className = className;
 return element;
}
export function link(text, href) {
 const element = node('a', text); element.href = href; return element;
}
export function failure(error) { document.querySelector('#status').textContent = error.message; }
