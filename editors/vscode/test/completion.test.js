const assert = require('node:assert/strict');
const test = require('node:test');
const { createCompletionProvider } = require('../extension');

class SnippetString {
  constructor() { this.value = ''; }
  appendText(text) { this.value += text; }
  appendPlaceholder(text) { this.value += `\${${text}}`; }
}

class Range {
  constructor(start, end) { Object.assign(this, { start, end }); }
}

class CompletionItem {
  constructor(label, kind) { Object.assign(this, { label, kind }); }
}

const provider = createCompletionProvider({
  CompletionItem,
  CompletionItemKind: { Function: 'function', Keyword: 'keyword', Variable: 'variable' },
  Range,
  SnippetString,
});

function complete(text, column = text.length) {
  return completeAt(text.slice(0, column) + '|' + text.slice(column));
}

function completeAt(source) {
  const offset = source.indexOf('|');
  assert.notEqual(offset, -1);
  const text = source.slice(0, offset) + source.slice(offset + 1);
  const before = text.slice(0, offset);
  const position = { line: before.split('\n').length - 1, character: offset - before.lastIndexOf('\n') - 1 };
  const line = text.split(/\r?\n/)[position.line];
  return provider.provideCompletionItems({
    getText: () => text,
    offsetAt: (point) => offset - position.character + point.character,
    lineAt: () => ({ text: line }),
    getWordRangeAtPosition() {
      const word = [...line.matchAll(/[A-Za-z_][A-Za-z0-9_]*/g)].find((candidate) =>
        candidate.index <= position.character && position.character <= candidate.index + candidate[0].length);
      return word && new Range({ line: position.line, character: word.index }, { line: position.line, character: word.index + word[0].length });
    },
  }, position);
}

test('settle and round expose different parameter types and editable arguments', () => {
  const [settle] = complete('let total = set');
  assert.equal(settle.label, 'settle');
  assert.equal(settle.detail, 'settle(value: Money, digits: Decimal, mode: String) → Money');
  assert.equal(settle.insertText.value, 'settle(${value}, ${digits}, ${mode})');
  assert.match(settle.documentation, /정산/);
  const [round] = complete('rou');
  assert.match(round.detail, /round\(value: Decimal/);
});

test('existing parentheses remain intact when a partial name is replaced', () => {
  const [settle] = complete('set("USD")', 3);
  assert.equal(settle.insertText, 'settle');
  assert.deepEqual(settle.range, new Range({ line: 0, character: 0 }, { line: 0, character: 3 }));
});

test('only keyword completions are offered for decimal input declarations', () => {
  const [decimal] = complete('  input annual_rate as dec');
  assert.equal(decimal.label, 'decimal');
  assert.equal(decimal.kind, 'keyword');
  assert.equal(decimal.insertText, 'decimal');
  const [parse] = complete('let amount = dec');
  assert.equal(parse.kind, 'function');
});

test('comments and quoted strings suppress the extension’s completions', () => {
  assert.deepEqual(complete('let x = 1; // set'), []);
  assert.deepEqual(complete('print("escaped \\" set'), []);
  assert.equal(complete('print("// text"); rou')[0].label, 'round');
});

test('rule keywords and policy-sensitive function docs match the interpreter', () => {
  assert.equal(complete('  requ')[0].kind, 'keyword');
  assert.match(complete('pri')[0].documentation, /-rule-only/);
  assert.match(complete('conv')[0].detail, /quote: FXQuote/);
});

test('only completed preceding let declarations appear at the cursor', () => {
  const [item] = completeAt('let principal = 10;\r\npri|;\r\nlet private_value = 20;').filter(i => i.kind === 'variable');
  assert.equal(item.label, 'principal');
  assert.equal(item.insertText, 'principal');
  assert.deepEqual(completeAt('let temporary = tem|;'), []);
  assert.deepEqual(completeAt('let total = round(1,\n tot|);'), []);
  assert.deepEqual(completeAt('let declared = 1;\nlet dec|'), []);
});

test('condition branches hide sibling and closed bindings, with nearest name taking priority', () => {
  const setup = 'let visible = 1; let round = fn(n) { return n; }; if (true) { let branch_only = 2; ';
  assert.equal(completeAt(setup + 'bra|; }')[0].label, 'branch_only');
  assert.equal(completeAt(setup + 'vis|; }')[0].label, 'visible');
  assert.deepEqual(completeAt(setup + '} else { bra|; }'), []);
  assert.deepEqual(completeAt(setup + '} bra|;'), []);
  const shadow = completeAt(setup + 'let round = 7; rou|; }');
  assert.deepEqual(shadow.map(i => [i.label, i.kind]), [['round', 'variable']]);
});

test('function snippets reflect parameters, preserve calls, and see enclosing bindings and recursion', () => {
  const declaration = 'let outer_value = 1; let interest = fn(principal, annual_rate) {';
  const parameter = completeAt(declaration + ' return pri|; }')[0];
  assert.equal(parameter.label, 'principal');
  assert.equal(parameter.kind, 'variable');
  assert.equal(completeAt(declaration + ' return outer_v|; }')[0].label, 'outer_value');
  const recursive = completeAt(declaration + ' return inte|; }')[0];
  assert.equal(recursive.kind, 'function');
  assert.equal(recursive.insertText.value, 'interest(${principal}, ${annual_rate})');
  const [call] = completeAt(declaration + ' return principal; }; inte|(1, 2);');
  assert.equal(call.insertText, 'interest');
  assert.deepEqual(completeAt(declaration + ' return principal; }; annual_r|;'), []);
  assert.deepEqual(completeAt('let immediate = fn() { return imme|; }();'), []);
  assert.equal(completeAt('let grouped = (fn() { return grou|; });')[0].kind, 'function');
});

test('nested functions inherit parameters while a local binding can shadow the recursive name', () => {
  const program = 'let combine = fn(first) { let inner = fn(second) { return fir| + second; }; };';
  assert.equal(completeAt(program)[0].label, 'first');
  assert.equal(completeAt('let combine = fn() { let combine = 1; return comb|; };')[0].kind, 'variable');
});

test('rule inputs and step declarations persist, but names of rule, steps, and output header do not bind', () => {
  const rule = 'rule interest {\n input principal in "KRW"\n input annual_rate as decimal\n output payable in "KRW"\n';
  const first = ' step calculate {\n  let intermediate = principal * annual_rate\n  if (true) { let hidden = 0 }\n }\n';
  assert.equal(completeAt(rule + ' step calculate {\n  let accrued = annual_r|\n }')[0].label, 'annual_rate');
  assert.equal(completeAt(rule + ' step calculate {\n  let accrued = principal\n  let next = accru|\n }')[0].label, 'accrued');
  assert.deepEqual(completeAt(rule + ' step calculate {\n  let accrued =\n    accru|\n }'), []);
  assert.deepEqual(completeAt(rule + ' step calculate {\n  let accrued = principal\n    + accru|\n }'), []);
  assert.deepEqual(completeAt(rule + ' step calculate {\n  let incomplete = fn\n  incom|\n }'), []);
  assert.deepEqual(completeAt(rule + ' step calculate {\n  let incomplete = "text\n  incom|\n }'), []);
  assert.equal(completeAt(rule + first + ' step settle_interest {\n  let payable = interm|\n }')[0].label, 'intermediate');
  assert.deepEqual(completeAt(rule + first + ' step settle_interest {\n  let payable = paya|\n }'), []);
  assert.deepEqual(completeAt(rule + first + ' step settle_interest {\n  let payable = hidd|\n }'), []);
  assert.equal(completeAt(rule + first + ' step settle_interest {\n  let payable = intermediate\n }\n return paya|')[0].label, 'payable');
});

test('strings and comments never create or close a scope', () => {
  const text = 'let found = 1; // let fake = 2; }\nlet text = "let fake = 3; {";\n';
  assert.deepEqual(completeAt(text + 'fak|;'), []);
  assert.equal(completeAt(text + 'foun|;')[0].label, 'found');
});
