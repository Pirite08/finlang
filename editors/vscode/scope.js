const words = /^[A-Za-z_][A-Za-z0-9_]*$/;
const reserved = new Set(['let', 'fn', 'return', 'if', 'else', 'true', 'false']);
const continuations = new Set(['(', '+', '-', '*', '==', '!=', '<', '>', '<=', '>=']);

function isName(token) {
  return token?.kind === 'word' && !reserved.has(token.value);
}

function tokenize(source) {
  const tokens = [];
  const parentheses = [];
  const braces = [];
  const pattern = /\/\/[^\r\n]*|"(?:\\[^\r\n]|[^"\\\r\n])*"?|[A-Za-z_][A-Za-z0-9_]*|[0-9]+(?:\.[0-9]+)?|==|!=|<=|>=|\S/g;
  let lastEnd = 0;
  for (const match of source.matchAll(pattern)) {
    const [value] = match;
    if (value.startsWith('//')) continue;
    const start = match.index;
    const end = start + value.length;
    const kind = words.test(value) ? 'word' : value.startsWith('"') ? 'string' : /^[0-9]/.test(value) ? 'number' : 'punctuation';
    const token = { value, kind, start, end, lineBreak: /[\r\n]/.test(source.slice(lastEnd, start)) };
    const index = tokens.length;
    tokens.push(token);
    lastEnd = end;
    if (value === '(') parentheses.push(index);
    if (value === '{') braces.push(index);
    const opener = value === ')' ? parentheses.pop() : value === '}' ? braces.pop() : undefined;
    if (opener !== undefined) {
      token.mate = opener;
      tokens[opener].mate = index;
    }
  }
  return tokens;
}

function functionParameters(tokens, open, close) {
  const parameters = [];
  for (let index = open + 1; index < close; index += 2) {
    if (!isName(tokens[index]) || (index + 1 < close && tokens[index + 1].value !== ',')) return null;
    parameters.push(tokens[index].value);
  }
  if (tokens[close - 1]?.value === ',') return null;
  return parameters;
}

// Recognize a bound function literal, including parentheses around it. A
// function called as part of the initializer does not bind a recursive name.
function functionLiteral(tokens, start) {
  let index = start;
  while (tokens[index]?.value === '(') index++;
  if (tokens[index]?.value !== 'fn' || tokens[index + 1]?.value !== '(') return null;
  const parameterEnd = tokens[index + 1].mate;
  if (parameterEnd === undefined) return null;
  const body = parameterEnd + 1;
  if (tokens[body]?.value !== '{') return null;
  const parameters = functionParameters(tokens, index + 1, parameterEnd);
  if (!parameters) return null;
  const bodyEnd = tokens[body].mate;
  if (bodyEnd === undefined) return { body, parameters, end: null };
  let end = bodyEnd + 1;
  for (let count = start; count < index; count++) {
    if (tokens[end]?.value !== ')') return null;
    end++;
  }
  return { body, parameters, end };
}

function frame(kind, { bindings = new Map(), inStep = false, self = null } = {}) {
  return { kind, bindings, inStep, self, pending: null, parentheses: [] };
}

function define(bindings, binding) {
  if (!bindings.has(binding.name)) bindings.set(binding.name, binding);
}

function canEndExpression(token) {
  if (token.kind === 'string') return /^"(?:\\[^\r\n]|[^"\\\r\n])*"$/.test(token.value);
  return isName(token) || token.kind === 'number' || ['true', 'false', ')', '}'].includes(token.value);
}

function finishBinding(current, tokens, end) {
  const pending = current.pending;
  const previous = tokens[end - 1];
  if (!pending || end <= pending.start || current.parentheses.length) return;
  if (!canEndExpression(previous)) return;
  const literal = functionLiteral(tokens, pending.start);
  const parameters = literal?.end === end ? literal.parameters : null;
  define(current.bindings, { name: pending.name, parameters, description: '현재 위치에서 사용할 수 있는 선언입니다.' });
  current.pending = null;
}

function inputBinding(tokens, start) {
  const [name, connector, type] = tokens.slice(start + 1, start + 4);
  if (!isName(name)) return null;
  if (connector?.value === 'as' && type?.value === 'decimal') {
    return { name: name.value, description: '규칙 입력 · Decimal' };
  }
  if (connector?.value === 'in' && type?.kind === 'string' && type.value.length >= 2 && type.value.endsWith('"')) {
    return { name: name.value, description: `규칙 입력 · Money ${type.value}` };
  }
  return null;
}

function openBlock(current, tokens, index) {
  const before = tokens[index - 1];
  const header = tokens[index - 2]?.value;
  if (index === 2 && header === 'rule' && isName(before)) return frame('rule');
  if (current.kind === 'rule' && header === 'step' && isName(before)) {
    return frame('step', { bindings: current.bindings, inStep: true });
  }

  const parameterOpen = before?.value === ')' ? before.mate : undefined;
  if (parameterOpen === undefined || tokens[parameterOpen - 1]?.value !== 'fn') {
    return frame('block', { inStep: current.inStep });
  }
  const parameters = functionParameters(tokens, parameterOpen, index - 1) ?? [];
  const literal = current.pending && functionLiteral(tokens, current.pending.start);
  const next = literal && tokens[literal.end];
  const standalone = !next || [';', '}'].includes(next.value) || (current.inStep && next.lineBreak && !continuations.has(next.value));
  const self = literal?.body === index && standalone && !current.bindings.has(current.pending.name)
    ? { name: current.pending.name, parameters, description: '현재 함수의 재귀 호출입니다.' } : null;
  const fn = frame('function', { self });
  for (const name of parameters) define(fn.bindings, { name, description: '함수 매개변수입니다.' });
  return fn;
}

function collectBindings(source, offset) {
  const tokens = tokenize(source);
  const frames = [frame('file')];
  let index = 0;
  for (; index < tokens.length && tokens[index].start < offset; index++) {
    const current = frames.at(-1);
    const { value, lineBreak } = tokens[index];
    if (current.inStep && lineBreak && !continuations.has(value)) finishBinding(current, tokens, index);

    if (value === 'let' && isName(tokens[index + 1]) && tokens[index + 2]?.value === '=') {
      current.pending = { name: tokens[index + 1].value, start: index + 3 };
    } else if (value === '(') {
      current.parentheses.push(index);
    } else if (value === ')') {
      current.parentheses.pop();
    } else if (value === ';') {
      finishBinding(current, tokens, index);
    } else if (value === '{') {
      frames.push(openBlock(current, tokens, index));
    } else if (value === '}' && frames.length > 1) {
      finishBinding(current, tokens, index);
      frames.pop();
    } else if (current.kind === 'rule' && value === 'input' && tokens[index + 3]?.start < offset) {
      const binding = inputBinding(tokens, index);
      if (binding) define(current.bindings, binding);
      index += 3;
    } else if (current.kind === 'rule' && value === 'output' && tokens[index + 3]?.start < offset) {
      if (isName(tokens[index + 1])) current.output = tokens[index + 1].value;
      index += 3;
    }
  }

  const current = frames.at(-1);
  const next = tokens[index];
  const lineBreakAtCursor = /[\r\n]/.test(source.slice(tokens[index - 1]?.end ?? 0, offset));
  if (current.inStep && lineBreakAtCursor && !continuations.has(next?.value)) finishBinding(current, tokens, index);

  const previous = tokens[index - 1]?.value;
  const parameterOpen = current.parentheses.at(-1);
  const inParameters = parameterOpen !== undefined && tokens[parameterOpen - 1]?.value === 'fn';
  if (previous === 'let' || inParameters || (current.kind === 'rule' && ['input', 'output', 'step'].includes(previous))) {
    return { bindings: new Map(), restricted: true };
  }
  if (current.kind === 'rule' && previous === 'return') {
    const binding = current.bindings.get(current.output);
    return { bindings: binding ? new Map([[binding.name, binding]]) : new Map(), restricted: true };
  }

  const bindings = new Map();
  for (const active of frames.reverse()) {
    for (const binding of active.bindings.values()) define(bindings, binding);
    if (active.self) define(bindings, active.self);
  }
  return { bindings, restricted: false };
}

module.exports = { collectBindings };
