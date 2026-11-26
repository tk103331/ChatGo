(function () {
  'use strict';

  var surfaces = Object.create(null);
  var root = document.getElementById('root');
  var pendingTimer = 0;
  var agentCalls = Object.create(null);
  var nextCall = 1;
  var uiState = Object.create(null);

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
    not: function (a) { return !a.value; },
    formatString: function (a, data) {
      return String(a.value == null ? '' : a.value).replace(/\$\{([^}]+)\}/g, function (_, expr) {
        var v;
        if (expr.charAt(0) === '@') v = callFunction(expr, {}, data);
        else if (expr.charAt(0) === '/') v = getPath(data, expr);
        else v = getPath(data, '/' + expr);
        return textOf(v);
      });
    },
    formatNumber: function (a) {
      var n = Number(a.value) || 0;
      var d = a.decimals == null ? 0 : Number(a.decimals);
      return n.toLocaleString(undefined, { minimumFractionDigits: d, maximumFractionDigits: d, useGrouping: a.grouping !== false });
    },
    formatCurrency: function (a) {
      var n = Number(a.value) || 0;
      try { return n.toLocaleString(undefined, { style: 'currency', currency: a.currency || 'USD' }); }
      catch (e) { return n.toFixed(2) + ' ' + (a.currency || ''); }
    },
    formatDate: function (a) {
      var d = new Date(a.value);
      if (isNaN(d.getTime())) return textOf(a.value);
      var f = String(a.format || 'yyyy-MM-dd');
      var pad = function (n) { return n < 10 ? '0' + n : String(n); };
      var months = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
      var days = ['Sun','Mon','Tue','Wed','Thu','Fri','Sat'];
      return f.replace(/yyyy|yy|MMMM|MMM|MM|M|dd|d|EEEE|E|HH|H|hh|h|mm|ss|a/g, function (t) {
        switch (t) {
          case 'yyyy': return String(d.getFullYear());
          case 'yy': return pad(d.getFullYear() % 100);
          case 'MMMM': return ['January','February','March','April','May','June','July','August','September','October','November','December'][d.getMonth()];
          case 'MMM': return months[d.getMonth()];
          case 'MM': return pad(d.getMonth() + 1);
          case 'M': return String(d.getMonth() + 1);
          case 'dd': return pad(d.getDate());
          case 'd': return String(d.getDate());
          case 'EEEE': return ['Sunday','Monday','Tuesday','Wednesday','Thursday','Friday','Saturday'][d.getDay()];
          case 'E': return days[d.getDay()];
          case 'HH': return pad(d.getHours());
          case 'H': return String(d.getHours());
          case 'hh': return pad(d.getHours() % 12 || 12);
          case 'h': return String(d.getHours() % 12 || 12);
          case 'mm': return pad(d.getMinutes());
          case 'ss': return pad(d.getSeconds());
          case 'a': return d.getHours() < 12 ? 'AM' : 'PM';
        }
        return t;
      });
    },
    pluralize: function (a) {
      var n = Number(a.value) || 0;
      if (n === 0 && a.zero) return a.zero;
      if (n === 1 && a.one) return a.one;
      if (n === 2 && a.two) return a.two;
      return a.other || '';
    }
  };

  function callFunction(name, args, data) {
    if (name === '@index') return data && data.__a2uiIndex;
    var fn = FUNCS[name];
    if (!fn) return undefined;
    return fn(args || {}, data);
  }

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
    if (path === '@index') return data && data.__a2uiIndex;
    if (typeof path === 'string' && path.charAt(0) !== '/') path = '/' + path;
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
      var call = v['@call'];
      var callName = typeof call === 'string' ? call : (call && call.name) || '';
      if (callName) {
        var args = resolve(v.args || {}, data) || {};
        return callFunction(callName, args, data);
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
      case 'Video':
        el = videoNode(s, comp);
        break;
      case 'AudioPlayer':
        el = audioNode(s, comp);
        break;
      case 'Tabs':
        el = tabsNode(s, comp, seen, depth);
        break;
      case 'Modal':
        el = modalNode(s, comp, seen, depth);
        break;
      case 'DateTimeInput':
        el = dateTimeNode(s, comp);
        break;
      case 'Icon':
        el = iconNode(s, comp);
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
    var children = comp.children;
    if (children && !Array.isArray(children) && children.componentId) {
      var list = getPath(s.data, children.path);
      if (Array.isArray(list)) {
        list.forEach(function (item, index) {
          var scoped = Object.assign({}, s, { data: item && typeof item === 'object' ? Object.assign({}, item, { __a2uiIndex: index }) : { __a2uiIndex: index } });
          var siblingSeen = Object.assign({}, seen);
          delete siblingSeen[children.componentId];
          el.appendChild(nodeOf(scoped, children.componentId, siblingSeen, depth + 1));
        });
      }
      return;
    }
    (children || []).forEach(function (id) {
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
    var event = action.event || action;
    if (!event.name) return;
    var rawContext = event.context || action.context || {};
    var context = {};
    if (Array.isArray(rawContext)) {
      rawContext.forEach(function (item) {
        if (item && item.key) context[item.key] = resolve(item.value, s.data);
      });
    } else {
      context = resolve(rawContext, s.data) || {};
    }
    if (!Object.keys(context).length && s.data && typeof s.data === 'object' && !Array.isArray(s.data)) {
      context = s.data;
    }
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
    input.addEventListener('pointerdown', function () { input.focus(); });
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

  function videoNode(s, comp) {
    var url = textOf(resolve(comp.url, s.data));
    if (!url) return placeholder('[视频]');
    var el = document.createElement('video');
    el.className = 'a2ui-video';
    el.controls = true;
    el.preload = 'metadata';
    if (comp.posterUrl) el.poster = textOf(resolve(comp.posterUrl, s.data));
    el.src = url;
    el.addEventListener('error', function () { el.replaceWith(placeholder('[视频加载失败] ' + url)); });
    return el;
  }

  function audioNode(s, comp) {
    var wrap = document.createElement('div');
    wrap.className = 'a2ui-audio';
    var description = textOf(resolve(comp.description, s.data));
    if (description) {
      var d = document.createElement('div');
      d.className = 'a2ui-muted';
      d.textContent = description;
      wrap.appendChild(d);
    }
    var url = textOf(resolve(comp.url, s.data));
    if (!url) {
      wrap.appendChild(placeholder('[音频]'));
      return wrap;
    }
    var el = document.createElement('audio');
    el.controls = true;
    el.src = url;
    el.addEventListener('error', function () { el.replaceWith(placeholder('[音频加载失败] ' + url)); });
    wrap.appendChild(el);
    return wrap;
  }

  function tabsNode(s, comp, seen, depth) {
    var tabs = comp.tabs || [];
    var key = s.id + ':tab:' + comp.id;
    var active = uiState[key] || 0;
    var wrap = document.createElement('div');
    wrap.className = 'a2ui-tabs';
    var bar = document.createElement('div');
    bar.className = 'a2ui-tabbar';
    tabs.forEach(function (tab, index) {
      var button = document.createElement('button');
      button.className = 'a2ui-tab' + (index === active ? ' active' : '');
      button.textContent = textOf(resolve(tab.title, s.data));
      button.addEventListener('click', function () {
        uiState[key] = index;
        render(s);
        reportSize();
      });
      bar.appendChild(button);
    });
    wrap.appendChild(bar);
    var panel = document.createElement('div');
    panel.className = 'a2ui-tabpanel';
    var current = tabs[active];
    if (current && current.child) panel.appendChild(nodeOf(s, current.child, seen, depth + 1));
    wrap.appendChild(panel);
    return wrap;
  }

  function modalNode(s, comp, seen, depth) {
    var key = s.id + ':modal:' + comp.id;
    var open = !!uiState[key];
    var wrap = document.createElement('div');
    wrap.className = 'a2ui-modal-wrap';
    var trigger = document.createElement('div');
    trigger.className = 'a2ui-modal-trigger';
    if (comp.trigger) trigger.appendChild(nodeOf(s, comp.trigger, seen, depth + 1));
    trigger.addEventListener('click', function () {
      uiState[key] = true;
      render(s);
      reportSize();
    });
    wrap.appendChild(trigger);
    if (open) {
      var overlay = document.createElement('div');
      overlay.className = 'a2ui-modal-overlay';
      var card = document.createElement('div');
      card.className = 'a2ui-modal-content';
      var close = document.createElement('button');
      close.className = 'a2ui-modal-close';
      close.textContent = '×';
      close.setAttribute('aria-label', 'Close');
      close.addEventListener('click', function (event) {
        event.stopPropagation();
        uiState[key] = false;
        render(s);
        reportSize();
      });
      card.appendChild(close);
      if (comp.content) card.appendChild(nodeOf(s, comp.content, seen, depth + 1));
      overlay.appendChild(card);
      overlay.addEventListener('click', function (event) {
        if (event.target === overlay) {
          uiState[key] = false;
          render(s);
          reportSize();
        }
      });
      wrap.appendChild(overlay);
    }
    return wrap;
  }

  function dateTimeNode(s, comp) {
    var wrap = document.createElement('div');
    wrap.className = 'a2ui-field';
    var label = textOf(resolve(comp.label, s.data));
    if (label) {
      var l = document.createElement('label');
      l.textContent = label;
      wrap.appendChild(l);
    }
    var input = document.createElement('input');
    var date = comp.enableDate !== false;
    var time = !!comp.enableTime;
    input.type = date && time ? 'datetime-local' : time ? 'time' : 'date';
    var path = comp.value && (comp.value['@path'] || comp.value.path);
    input.value = textOf(resolve(comp.value, s.data));
    if (comp.min) input.min = textOf(resolve(comp.min, s.data));
    if (comp.max) input.max = textOf(resolve(comp.max, s.data));
    input.addEventListener('change', function () {
      if (path) s.data = setPath(s.data, path, input.value);
      reportSize();
    });
    wrap.appendChild(input);
    return wrap;
  }

  function imageNode(s, comp) {
    var url = textOf(resolve(comp.url || comp.src, s.data));
    if (!url) return placeholder('[图片]');
    var img = document.createElement('img');
    img.className = 'a2ui-img';
    var fit = String(comp.fit || '').toLowerCase();
    if (fit === 'cover') img.style.objectFit = 'cover';
    else if (fit === 'contain') img.style.objectFit = 'contain';
    else if (fit === 'none') img.style.objectFit = 'none';
    else if (fit === 'scaledown') img.style.objectFit = 'scale-down';
    img.src = url;
    img.alt = textOf(resolve(comp.description, s.data)) || textOf(resolve(comp.alt, s.data)) || '';
    img.addEventListener('error', function () { img.replaceWith(placeholder('[图片加载失败] ' + url)); });
    return img;
  }

  var ICON_PATHS = {
    check: 'M20 6L9 17l-5-5',
    close: 'M18 6L6 18M6 6l12 12',
    add: 'M12 5v14M5 12h14',
    search: 'M21 21l-4.35-4.35M17 11a6 6 0 11-12 0 6 6 0 0112 0z',
    home: 'M3 10.5L12 3l9 7.5V21H3z',
    mail: 'M4 5h16v14H4zM4 6l8 7 8-7',
    person: 'M12 12a4 4 0 100-8 4 4 0 000 8zM4 21c0-4 4-6 8-6s8 2 8 6',
    star: 'M12 3l2.8 5.7 6.2.9-4.5 4.4 1.1 6.2L12 17.3 6.4 20l1.1-6.2L3 9.6l6.2-.9z',
    warning: 'M12 3L2 21h20zM12 9v6M12 18v1',
    info: 'M12 22a10 10 0 100-20 10 10 0 000 20zM12 11v6M12 7v1',
    settings: 'M12 15.5a3.5 3.5 0 100-7 3.5 3.5 0 000 7z',
    edit: 'M4 20h4l10-10-4-4L4 16z',
    delete: 'M4 7h16M9 7V4h6v3M7 7l1 14h8l1-14',
    download: 'M12 3v12M7 10l5 5 5-5M4 21h16',
    upload: 'M12 21V9M7 14l5-5 5 5M4 3h16',
    arrowBack: 'M19 12H5M12 19l-7-7 7-7',
    arrowForward: 'M5 12h14M12 5l7 7-7 7',
    play: 'M6 4l14 8-14 8z',
    pause: 'M7 4h4v16H7zM13 4h4v16h-4z',
    favorite: 'M12 3l2.8 5.7 6.2.9-4.5 4.4 1.1 6.2L12 17.3 6.4 20l1.1-6.2L3 9.6l6.2-.9z',
    calendarToday: 'M4 6h16v15H4zM8 3v5M16 3v5M4 10h16',
    locationOn: 'M12 22s8-6 8-13a8 8 0 10-16 0c0 7 8 13 8 13zM12 11a2.5 2.5 0 100-5 2.5 2.5 0 000 5z',
    menu: 'M4 7h16M4 12h16M4 17h16',
    moreVert: 'M12 6h.01M12 12h.01M12 18h.01',
    send: 'M22 2L11 13M22 2l-7 20-4-9-9-4z'
  };

  function iconNode(s, comp) {
    var name = textOf(resolve(comp.name, s.data));
    var svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('width', '18');
    svg.setAttribute('height', '18');
    svg.setAttribute('viewBox', '0 0 24 24');
    svg.setAttribute('fill', 'none');
    svg.setAttribute('stroke', 'currentColor');
    svg.setAttribute('stroke-width', '2');
    svg.setAttribute('stroke-linecap', 'round');
    svg.setAttribute('stroke-linejoin', 'round');
    svg.style.verticalAlign = 'middle';
    var path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
    var d = ICON_PATHS[name];
    if (!d && comp.name && typeof comp.name === 'object' && comp.name.svgPath) d = textOf(resolve(comp.name.svgPath, s.data));
    path.setAttribute('d', d || 'M12 2a10 10 0 100 20 10 10 0 000-20z');
    svg.appendChild(path);
    return svg;
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
