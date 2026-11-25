(function () {
  'use strict';

  var surfaces = Object.create(null);
  var root = document.getElementById('root');
  var pendingTimer = 0;
  var agentCalls = Object.create(null);
  var nextCall = 1;

  var FUNCS = {
    required: function (a) { return textOf(a.value) !== ''; },
    length: function (a) { return textOf(a.value).length; },
    numeric: function (a) { return !isNaN(Number(textOf(a.value))); },
    email: function (a) { var v = textOf(a.value); return v.indexOf('@') > 0 && v.indexOf('.') > 0; },
    regex: function (a) {
      try { return new RegExp(textOf(a.pattern)).test(textOf(a.value)); } catch (e) { return false; }
    },
    and: function (a) { for (var k in a) if (a[k] === false) return false; return true; },
    or: function (a) { for (var k in a) if (a[k] === true) return true; return false; },
    not: function (a) { return !a.value; }
  };

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  function tokens(path) {
    if (path === '' || path === '/') return [];
    if (path.charAt(0) !== '/') return [];
    return path.slice(1).split('/').map(function (p) {
      return p.replace(/~1/g, '/').replace(/~0/g, '~');
    });
  }

  function getPath(data, path) {
    var cur = data;
    var parts = tokens(path);
    for (var i = 0; i < parts.length; i++) {
      if (cur == null) return undefined;
      cur = cur[parts[i]];
    }
    return cur;
  }

  function setPath(data, path, value) {
    var parts = tokens(path);
    if (!parts.length) return value;
    var cur = data;
    for (var i = 0; i < parts.length - 1; i++) {
      if (cur == null || typeof cur !== 'object') return data;
      if (cur[parts[i]] == null) cur[parts[i]] = {};
      cur = cur[parts[i]];
    }
    if (cur && typeof cur === 'object') cur[parts[parts.length - 1]] = value;
    return data;
  }

  function resolve(v, data) {
    if (v && typeof v === 'object') {
      if (Object.prototype.hasOwnProperty.call(v, '@path')) return getPath(data, v['@path']);
      if (Object.prototype.hasOwnProperty.call(v, 'path') && typeof v.path === 'string') return getPath(data, v.path);
      var literals = ['literalString', 'literalNumber', 'literalBoolean', 'literalArray', 'literalObject', 'literal'];
      for (var i = 0; i < literals.length; i++) {
        if (Object.prototype.hasOwnProperty.call(v, literals[i])) return v[literals[i]];
      }
      if (Array.isArray(v)) return v.map(function (x) { return resolve(x, data); });
      var out = {};
      for (var k in v) if (Object.prototype.hasOwnProperty.call(v, k)) out[k] = resolve(v[k], data);
      return out;
    }
    return v;
  }

  function normalize(comp) {
    if (!comp || typeof comp !== 'object') return comp;
    var out = {};
    for (var k in comp) if (Object.prototype.hasOwnProperty.call(comp, k)) out[k] = comp[k];
    if (out.component && typeof out.component === 'object') {
      var name = Object.keys(out.component)[0];
      var props = out.component[name] || {};
      for (var p in props) if (Object.prototype.hasOwnProperty.call(props, p)) out[p] = props[p];
      out.component = name;
    }
    if (out.children && !Array.isArray(out.children) && Array.isArray(out.children.explicitList)) {
      out.children = out.children.explicitList;
    }
    return out;
  }

  function textOf(v) {
    if (v == null) return '';
    if (typeof v === 'string') return v;
    if (typeof v === 'number' || typeof v === 'boolean') return String(v);
    return JSON.stringify(v);
  }

  function surfaceOf(id) {
    var s = surfaces[id];
    if (!s) {
      s = surfaces[id] = { id: id, components: Object.create(null), data: {}, sendDataModel: false, catalogId: '' };
    }
    return s;
  }

  function apply(msg) {
    if (!msg || typeof msg !== 'object') return;
    if (msg.createSurface) {
      var c = msg.createSurface;
      var s = surfaceOf(c.surfaceId);
      s.catalogId = c.catalogId || '';
      s.sendDataModel = !!c.sendDataModel;
      s.components = Object.create(null);
      s.data = c.dataModel || {};
      (c.components || []).forEach(function (comp) {
        if (comp && comp.id) s.components[comp.id] = normalize(comp);
      });
      render(s);
      return;
    }
    if (msg.updateComponents) {
      var u = msg.updateComponents;
      var su = surfaceOf(u.surfaceId);
      (u.components || []).forEach(function (comp) {
        if (comp && comp.id) su.components[comp.id] = normalize(comp);
      });
      render(su);
      return;
    }
    if (msg.updateDataModel) {
      var d = msg.updateDataModel;
      var sd = surfaceOf(d.surfaceId);
      var path = d.path || '/';
      if (path === '/' || path === '') sd.data = d.value;
      else sd.data = setPath(sd.data, path, d.value);
      render(sd);
      return;
    }
    if (msg.deleteSurface) {
      delete surfaces[msg.deleteSurface.surfaceId];
      renderAll();
      return;
    }
    if (msg.agentFunctionResponse) {
      var r = msg.agentFunctionResponse;
      var pending = agentCalls[r.functionCallId];
      if (pending) {
        delete agentCalls[r.functionCallId];
        if (r.error) pending.reject(new Error(r.error.message || 'agent function failed'));
        else pending.resolve(r.value);
      }
      return;
    }
    if (msg.callRendererFunction) {
      callRendererFunction(msg.callRendererFunction);
    }
  }

  function callRendererFunction(call) {
    var cf = call.callFunction || {};
    var name = typeof cf['@call'] === 'string' ? cf['@call'] : (cf['@call'] && cf['@call'].name) || '';
    var surface = surfaces[call.surfaceId] || surfaces[Object.keys(surfaces)[0]];
    var data = surface ? surface.data : {};
    var args = resolve(cf.args || {}, data) || {};
    var response = { functionCallId: call.functionCallId };
    if (!name || !FUNCS[name]) {
      response.error = { code: 'INVALID_FUNCTION_CALL', message: 'unknown renderer function: ' + name };
    } else {
      try { response.value = FUNCS[name](args); }
      catch (e) { response.error = { code: 'INVALID_FUNCTION_CALL', message: String(e) }; }
    }
    postRendererMessage({ version: '1.0', rendererFunctionResponse: response });
  }

  function postRendererMessage(message) {
    post({ type: 'rendererMessage', rendererMessage: message });
  }

  window.a2ui = window.a2ui || {};
  window.a2ui.callAgentFunction = function (name, args) {
    var surfaceId = Object.keys(surfaces)[0] || '';
    var callId = 'rfn-' + (nextCall++);
    var promise = new Promise(function (resolve, reject) { agentCalls[callId] = { resolve: resolve, reject: reject }; });
    postRendererMessage({
      version: '1.0',
      callAgentFunction: {
        functionCallId: callId,
        surfaceId: surfaceId,
        callFunction: { '@call': name, catalogId: 'https://a2ui.org/specification/v1_0/catalogs/basic/catalog.json', args: args || {} }
      }
    });
    return promise;
  };

  function renderAll() {
    root.innerHTML = '';
    Object.keys(surfaces).forEach(function (id) { render(surfaces[id]); });
    reportSize();
  }

  function render(s) {
    root.innerHTML = '';
    var seen = Object.create(null);
    root.appendChild(nodeOf(s, 'root', seen, 0));
    reportSize();
  }

  function nodeOf(s, id, seen, depth) {
    if (depth > 64 || seen[id]) return placeholder('组件嵌套过深：' + id);
    seen[id] = true;
    var comp = normalize(s.components[id]);
    if (!comp) return placeholder('缺少组件：' + id);
    var type = comp.component || '';
    var el;
    switch (type) {
      case 'Text': case 'Title': case 'Caption': case 'Label':
        el = textNode(s, comp, type);
        break;
      case 'Row': case 'Column': case 'Box':
        el = container(s, comp, type, seen, depth);
        break;
      case 'Card': case 'Section':
        el = document.createElement('div');
        el.className = 'a2ui-card a2ui-col';
        childrenInto(s, comp, el, seen, depth);
        break;
      case 'List':
        el = container(s, comp, 'Column', seen, depth);
        break;
      case 'Divider':
        el = document.createElement('div');
        el.className = 'a2ui-divider';
        break;
      case 'Button':
        el = buttonNode(s, comp);
        break;
      case 'TextField': case 'TextArea':
        el = fieldNode(s, comp);
        break;
      case 'CheckBox': case 'Switch':
        el = checkNode(s, comp);
        break;
      case 'Slider':
        el = sliderNode(s, comp);
        break;
      case 'Select': case 'ChoicePicker': case 'RadioGroup': case 'SegmentedControl':
        el = choiceNode(s, comp);
        break;
      case 'Image':
        el = imageNode(s, comp);
        break;
      case 'Icon':
        el = placeholder('[图标] ' + textOf(resolve(comp.name, s.data)));
        break;
      default:
        el = placeholder('暂不支持：' + type);
    }
    return el;
  }

  function textNode(s, comp, type) {
    var value = textOf(resolve(comp.text, s.data));
    var el = document.createElement(type === 'Title' ? 'h3' : type === 'Caption' || type === 'Label' ? 'div' : 'div');
    var m = /^(#{1,6})\s+(.*)$/.exec(value);
    if (m) {
      var n = m[1].length;
      el.className = n <= 3 ? 'a2ui-h' + n : 'a2ui-h3';
      el.textContent = m[2];
    } else {
      el.textContent = value;
      var variant = String(comp.variant || comp.usageHint || '').toLowerCase();
      if (variant === 'h1') el.className = 'a2ui-h1';
      else if (variant === 'h2') el.className = 'a2ui-h2';
      else if (variant === 'h3' || type === 'Title') el.className = 'a2ui-h3';
      else if (variant === 'caption' || type === 'Caption' || type === 'Label') el.className = 'a2ui-muted';
    }
    return el;
  }

  function container(s, comp, type, seen, depth) {
    var el = document.createElement('div');
    el.className = type === 'Row' ? 'a2ui-row' : 'a2ui-col';
    if (comp.align === 'center') el.style.alignItems = 'center';
    if (comp.align === 'end') el.style.alignItems = 'flex-end';
    if (comp.align === 'stretch') el.style.alignItems = 'stretch';
    if (comp.justify === 'center') el.style.justifyContent = 'center';
    if (comp.justify === 'end') el.style.justifyContent = 'flex-end';
    if (comp.justify === 'spaceBetween') el.style.justifyContent = 'space-between';
    if (typeof comp.gap === 'number') el.style.gap = comp.gap + 'px';
    childrenInto(s, comp, el, seen, depth);
    return el;
  }

  function childrenInto(s, comp, el, seen, depth) {
    if (comp.child) el.appendChild(nodeOf(s, comp.child, seen, depth + 1));
    (comp.children || []).forEach(function (id) {
      el.appendChild(nodeOf(s, id, seen, depth + 1));
    });
  }

  function buttonNode(s, comp) {
    var el = document.createElement('button');
    var label = textOf(resolve(comp.label, s.data));
    if (!label && comp.child) {
      var child = normalize(s.components[comp.child]);
      if (child) label = textOf(resolve(child.text, s.data));
    }
    if (!label) label = '按钮';
    el.className = 'a2ui-btn';
    var variant = String(comp.variant || '').toLowerCase();
    if (variant === 'primary' || variant === 'filled' || !variant) el.className += ' primary';
    el.textContent = label;
    el.addEventListener('click', function () { sendAction(s, comp); });
    return el;
  }

  function sendAction(s, comp) {
    var action = comp.action || {};
    var event = action.event || {};
    if (!event.name) return;
    var context = resolve(event.context || {}, s.data) || {};
    var msg = {
      version: '1.0',
      action: {
        version: '1.0',
        name: event.name,
        surfaceId: s.id,
        sourceComponentId: comp.id,
        timestamp: new Date().toISOString(),
        userMessage: event.userMessage || '',
        context: context
      }
    };
    post({ type: 'action', action: msg.action, dataModel: s.data });
  }

  function fieldNode(s, comp) {
    var wrap = document.createElement('div');
    wrap.className = 'a2ui-field';
    var label = textOf(resolve(comp.label, s.data));
    if (label) {
      var l = document.createElement('label');
      l.textContent = label;
      wrap.appendChild(l);
    }
    var input = document.createElement('input');
    var variant = String(comp.variant || '').toLowerCase();
    if (comp.component === 'TextArea' || variant === 'longtext') {
      input = document.createElement('textarea');
      input.rows = 3;
    } else if (variant === 'obscured' || comp.obscured) {
      input.type = 'password';
    } else {
      input.type = 'text';
    }
    var path = comp.value && comp.value['@path'];
    input.value = textOf(resolve(comp.value, s.data));
    if (comp.placeholder) input.placeholder = comp.placeholder;
    input.addEventListener('input', function () {
      if (path) s.data = setPath(s.data, path, input.value);
    });
    wrap.appendChild(input);
    return wrap;
  }

  function checkNode(s, comp) {
    var wrap = document.createElement('label');
    wrap.className = 'a2ui-check';
    var input = document.createElement('input');
    input.type = 'checkbox';
    input.checked = !!resolve(comp.value, s.data);
    var path = comp.value && comp.value['@path'];
    input.addEventListener('change', function () {
      if (path) s.data = setPath(s.data, path, input.checked);
    });
    wrap.appendChild(input);
    var span = document.createElement('span');
    span.textContent = textOf(resolve(comp.label, s.data));
    wrap.appendChild(span);
    return wrap;
  }

  function sliderNode(s, comp) {
    var wrap = document.createElement('div');
    wrap.className = 'a2ui-field';
    var label = textOf(resolve(comp.label, s.data));
    if (label) {
      var l = document.createElement('label');
      l.textContent = label;
      wrap.appendChild(l);
    }
    var input = document.createElement('input');
    input.type = 'range';
    input.min = typeof comp.min === 'number' ? comp.min : 0;
    input.max = typeof comp.max === 'number' ? comp.max : 100;
    input.step = typeof comp.step === 'number' ? comp.step : 1;
    input.value = Number(resolve(comp.value, s.data)) || 0;
    var path = comp.value && comp.value['@path'];
    input.addEventListener('input', function () {
      if (path) s.data = setPath(s.data, path, Number(input.value));
    });
    wrap.appendChild(input);
    return wrap;
  }

  function choiceNode(s, comp) {
    var wrap = document.createElement('div');
    wrap.className = 'a2ui-col';
    var label = textOf(resolve(comp.label, s.data));
    if (label) {
      var l = document.createElement('div');
      l.className = 'a2ui-muted';
      l.textContent = label;
      wrap.appendChild(l);
    }
    var options = comp.options || [];
    var path = comp.value && comp.value['@path'];
    var current = resolve(comp.value, s.data);
    var multi = String(comp.variant || '').toLowerCase() === 'multipleselection' || Array.isArray(current);

    if (comp.component === 'Select' && !multi) {
      var sel = document.createElement('select');
      options.forEach(function (o) {
        var opt = document.createElement('option');
        opt.value = textOf(o.value);
        opt.textContent = textOf(o.label != null ? o.label : o.value);
        if (opt.value === textOf(current)) opt.selected = true;
        sel.appendChild(opt);
      });
      sel.addEventListener('change', function () {
        var chosen = options.find(function (o) { return textOf(o.value) === sel.value; });
        if (path) s.data = setPath(s.data, path, chosen ? chosen.value : sel.value);
      });
      wrap.appendChild(sel);
      return wrap;
    }

    options.forEach(function (o) {
      var lab = document.createElement('label');
      lab.className = 'a2ui-check';
      var input = document.createElement('input');
      input.type = multi ? 'checkbox' : 'radio';
      input.name = 'choice-' + comp.id;
      var val = textOf(o.value);
      var list = Array.isArray(current) ? current.map(textOf) : [];
      input.checked = multi ? list.indexOf(val) >= 0 : textOf(current) === val;
      input.addEventListener('change', function () {
        if (!path) return;
        if (multi) {
          var latest = Array.isArray(resolve(comp.value, s.data)) ? resolve(comp.value, s.data).slice() : [];
          var idx = latest.map(textOf).indexOf(val);
          if (input.checked && idx < 0) latest.push(o.value);
          if (!input.checked && idx >= 0) latest.splice(idx, 1);
          s.data = setPath(s.data, path, latest);
        } else {
          s.data = setPath(s.data, path, o.value);
        }
      });
      input.value = val;
      lab.appendChild(input);
      var span = document.createElement('span');
      span.textContent = textOf(o.label != null ? o.label : o.value);
      lab.appendChild(span);
      wrap.appendChild(lab);
    });
    return wrap;
  }

  function cssEscape(s) { return String(s).replace(/"/g, '\\"'); }

  function imageNode(s, comp) {
    var url = textOf(resolve(comp.url || comp.src, s.data));
    if (!url) return placeholder('[图片]');
    var img = document.createElement('img');
    img.className = 'a2ui-img';
    img.src = url;
    img.alt = comp.alt || '';
    img.addEventListener('error', function () { img.replaceWith(placeholder('[图片加载失败] ' + url)); });
    return img;
  }

  function placeholder(text) {
    var el = document.createElement('div');
    el.className = 'a2ui-placeholder';
    el.textContent = text;
    return el;
  }

  function reportError(message) {
    post({ type: 'error', message: String(message) });
  }

  var lastHeight = -1;
  function reportSize() {
    requestAnimationFrame(function () {
      var height = Math.ceil(document.documentElement.scrollHeight);
      if (height !== lastHeight) {
        lastHeight = height;
        post({ type: 'resize', width: window.innerWidth, height: height });
      }
    });
  }

  var pumping = false;
  function poll() {
    if (pumping || document.hidden) return;
    pumping = true;
    fetch('pending').then(function (res) {
      if (res.status === 204) return null;
      if (!res.ok) throw new Error('pending ' + res.status);
      return res.json();
    }).then(function (msg) {
      pumping = false;
      if (msg) {
        try { apply(msg); } catch (e) { reportError(e && e.message ? e.message : e); }
      }
      if (!document.hidden) setTimeout(poll, 0);
    }).catch(function (err) {
      pumping = false;
      reportError(err && err.message ? err.message : err);
      if (!document.hidden) setTimeout(poll, 1000);
    });
  }

  function post(body) {
    return fetch('rpc', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(body)
    }).catch(function () {});
  }

  document.addEventListener('visibilitychange', function () { if (!document.hidden) poll(); });
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () { poll(); reportSize(); });
  } else {
    poll();
    reportSize();
  }
  new ResizeObserver(reportSize).observe(document.documentElement);
})();
