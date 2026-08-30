(function () {
  'use strict';

  var THEME_KEY = 'daohang:theme';
  var root = document.documentElement;

  /* 窄屏时的列数上限，避免手动设置的大列数把卡片挤变形 */
  function capFor(w) {
    if (w < 420) return 2;
    if (w < 768) return 3;
    if (w < 1200) return 6;
    return 12;
  }

  /* ---------- 主题切换 ---------- */

  var themeBtn = document.getElementById('theme-toggle');

  function currentTheme() {
    return root.dataset.theme === 'dark' ? 'dark' : 'light';
  }

  function applyTheme(theme) {
    root.dataset.theme = theme === 'dark' ? 'dark' : 'light';
  }

  if (themeBtn) {
    themeBtn.addEventListener('click', function () {
      var next = currentTheme() === 'dark' ? 'light' : 'dark';
      applyTheme(next);
      try { localStorage.setItem(THEME_KEY, next); } catch (e) { /* 忽略 */ }
    });
  }

  /* ---------- 列数响应式 ---------- */
  /* 列数由后台配置写入 data-cols；此处仅在视口变化时重算上限。
     若后台配置为自适应（无 data-cols），则无需处理。 */

  function reCapCols() {
    var raw = root.dataset.cols;
    if (!raw) return;
    var n = Math.min(parseInt(raw, 10), capFor(window.innerWidth));
    if (isNaN(n) || n < 1) return;
    root.dataset.cols = String(n);
    root.style.setProperty('--cols', String(n));
  }

  var resizeTimer = null;
  window.addEventListener('resize', function () {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(reCapCols, 150);
  });

  /* ---------- 搜索框展开/收起 ---------- */

  var searchBox = document.getElementById('search-box');
  var searchBtn = document.getElementById('search-btn');

  function openSearch() {
    if (!searchBox) return;
    searchBox.hidden = false;
    if (searchBtn) searchBtn.setAttribute('aria-expanded', 'true');
    var input = searchBox.querySelector('input');
    if (input) input.focus();
  }
  function closeSearch() {
    if (!searchBox) return;
    searchBox.hidden = true;
    if (searchBtn) searchBtn.setAttribute('aria-expanded', 'false');
  }

  if (searchBtn) {
    searchBtn.addEventListener('click', function (e) {
      e.stopPropagation();
      if (searchBox && !searchBox.hidden) closeSearch();
      else openSearch();
    });
  }

  document.addEventListener('click', function (e) {
    if (!searchBox || searchBox.hidden) return;
    if (!searchBox.contains(e.target) && searchBtn && !searchBtn.contains(e.target)) {
      closeSearch();
    }
  });

  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape' && searchBox && !searchBox.hidden) closeSearch();
  });

  /* ---------- 搜索过滤 ---------- */

  var search = document.getElementById('search');
  var noResult = document.getElementById('no-result');

  if (search) {
    search.addEventListener('input', function () {
      var q = search.value.trim().toLowerCase();
      var total = 0;

      var secs = document.querySelectorAll('[data-category]');
      for (var i = 0; i < secs.length; i++) {
        var sec = secs[i];
        var visible = 0;
        var items = sec.querySelectorAll('.link-item');
        for (var j = 0; j < items.length; j++) {
          var item = items[j];
          var hay = ((item.getAttribute('data-title') || '') + ' ' +
                     (item.getAttribute('data-desc') || '') + ' ' +
                     (item.getAttribute('data-url') || '')).toLowerCase();
          var hit = !q || hay.indexOf(q) !== -1;
          item.hidden = !hit;
          if (hit) visible++;
        }
        sec.hidden = q !== '' && visible === 0;
        total += visible;
      }

      if (noResult) noResult.hidden = !(q !== '' && total === 0);
    });
  }

  /* ---------- 图标加载失败降级为首字母色块 ---------- */

  function toFallback(img) {
    if (!img.parentNode) return;
    var span = document.createElement('span');
    span.className = 'link-icon link-icon-fallback';
    span.style.setProperty('--h', img.getAttribute('data-hue') || '210');
    span.textContent = img.getAttribute('data-initial') || '#';
    img.parentNode.replaceChild(span, img);
  }

  var icons = document.querySelectorAll('img.link-icon');
  for (var k = 0; k < icons.length; k++) {
    (function (img) {
      img.addEventListener('error', function () { toFallback(img); }, { once: true });
      if (img.complete && img.naturalWidth === 0) toFallback(img);
    })(icons[k]);
  }

  /* ---------- 右下角浮动按钮：返回顶部 + 切换列表模式 ---------- */

  var backTopBtn = document.getElementById('back-top-btn');
  var layoutBtn = document.getElementById('layout-toggle-btn');
  var sidebar = document.getElementById('category-sidebar');
  var content = document.getElementById('category-content');

  // 返回顶部：滚动超过一屏才显示
  if (backTopBtn) {
    function toggleBackTop() {
      backTopBtn.hidden = window.scrollY < window.innerHeight;
    }
    window.addEventListener('scroll', toggleBackTop, { passive: true });
    toggleBackTop();
    backTopBtn.addEventListener('click', function () {
      window.scrollTo({ top: 0, behavior: 'smooth' });
    });
  }

  // 切换列表模式
  var LAYOUT_KEY = 'daohang:layout';
  function applyLayout(mode) {
    if (!sidebar || !content) return;
    var isSidebar = mode === 'sidebar';
    document.body.classList.toggle('layout-sidebar', isSidebar);
    sidebar.hidden = !isSidebar;

    var cats = content.querySelectorAll('.category');
    if (isSidebar) {
      // 侧边栏模式：只显示 active 分类
      for (var i = 0; i < cats.length; i++) {
        cats[i].classList.toggle('active', i === 0);
      }
      var navItems = sidebar.querySelectorAll('.cat-nav-item');
      for (var n = 0; n < navItems.length; n++) {
        navItems[n].classList.toggle('active', n === 0);
      }
    } else {
      // 默认模式：全部显示
      for (var j = 0; j < cats.length; j++) cats[j].classList.remove('active');
    }
  }

  if (layoutBtn) {
    layoutBtn.addEventListener('click', function () {
      var next = document.body.classList.contains('layout-sidebar') ? 'default' : 'sidebar';
      applyLayout(next);
      try { localStorage.setItem(LAYOUT_KEY, next); } catch (e) { /* 忽略 */ }
    });
  }

  // 侧边栏导航点击切换分类
  if (sidebar) {
    sidebar.addEventListener('click', function (e) {
      var item = e.target.closest('.cat-nav-item');
      if (!item || !content) return;
      e.preventDefault();
      var idx = item.getAttribute('data-cat-index');
      var navItems = sidebar.querySelectorAll('.cat-nav-item');
      var cats = content.querySelectorAll('.category');
      for (var n = 0; n < navItems.length; n++) {
        navItems[n].classList.toggle('active', navItems[n] === item);
      }
      for (var c = 0; c < cats.length; c++) {
        cats[c].classList.toggle('active', String(c) === idx);
      }
      window.scrollTo({ top: 0, behavior: 'smooth' });
    });
  }

  // 初始化：恢复上次的布局模式（默认 default）
  try {
    var savedLayout = localStorage.getItem(LAYOUT_KEY);
    if (savedLayout === 'sidebar') applyLayout('sidebar');
  } catch (e) { /* 忽略 */ }
})();
