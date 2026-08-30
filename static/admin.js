(function () {
  'use strict';

  /* 后台路径前缀，由模板通过 body 上的 data-admin-base 注入（可自定义，默认 /admin） */
  var ADMIN_BASE = document.body.getAttribute('data-admin-base') || '/admin';

  function byId(id) { return document.getElementById(id); }

  /* ---------- 后台搜索：提交后跳转并停留在对应 tab ---------- */
  function submitSearchForm(form) {
    var tab = form.getAttribute('data-tab');
    var params = [];
    var q = form.querySelector('input[type="search"]');
    if (q && q.value) params.push(q.name + '=' + encodeURIComponent(q.value));
    var cat = form.querySelector('select[name="link_cat"]');
    if (cat && cat.value) params.push('link_cat=' + encodeURIComponent(cat.value));
    window.location.href = ADMIN_BASE + (params.length ? '?' + params.join('&') : '') + '#' + tab;
  }

  var searchForms = document.querySelectorAll('.search-form[data-tab]');
  for (var sf = 0; sf < searchForms.length; sf++) {
    (function (form) {
      form.addEventListener('submit', function (e) {
        e.preventDefault();
        submitSearchForm(form);
      });
    })(searchForms[sf]);
  }

  /* ---------- 网址：分类筛选下拉自动提交 ---------- */
  var linkFilterForm = document.querySelector('.link-filter-form');
  if (linkFilterForm) {
    var catSelect = linkFilterForm.querySelector('select[name="link_cat"]');
    if (catSelect) {
      catSelect.addEventListener('change', function () {
        submitSearchForm(linkFilterForm);
      });
    }
  }

  /* ---------- 导入收藏：全选（全局 + 分组） ---------- */
  var selectAll = byId('select-all');
  var importChecks = document.querySelectorAll('.import-check');

  function syncGlobalCheck() {
    if (!selectAll) return;
    var all = importChecks.length > 0;
    for (var k = 0; k < importChecks.length; k++) {
      if (!importChecks[k].checked) { all = false; break; }
    }
    selectAll.checked = all;
  }

  function syncGroupCheck(groupIdx) {
    var box = document.querySelector('.group-select-all[data-group="' + groupIdx + '"]');
    if (!box) return;
    var gs = document.querySelectorAll('.import-check[data-group="' + groupIdx + '"]');
    var all = gs.length > 0;
    for (var k = 0; k < gs.length; k++) {
      if (!gs[k].checked) { all = false; break; }
    }
    box.checked = all;
  }

  // 全局全选
  if (selectAll && importChecks.length) {
    selectAll.addEventListener('change', function () {
      for (var i = 0; i < importChecks.length; i++) {
        importChecks[i].checked = selectAll.checked;
      }
      // 同步所有分组全选框
      var gboxes = document.querySelectorAll('.group-select-all');
      for (var g = 0; g < gboxes.length; g++) gboxes[g].checked = selectAll.checked;
    });
  }

  // 分组全选
  var groupBoxes = document.querySelectorAll('.group-select-all');
  for (var b = 0; b < groupBoxes.length; b++) {
    (function (box) {
      box.addEventListener('change', function () {
        var gi = box.getAttribute('data-group');
        var gs = document.querySelectorAll('.import-check[data-group="' + gi + '"]');
        for (var i = 0; i < gs.length; i++) gs[i].checked = box.checked;
        syncGlobalCheck();
      });
    })(groupBoxes[b]);
  }

  // 单个书签勾选变化时，同步全局和分组全选框状态
  if (importChecks.length) {
    for (var c = 0; c < importChecks.length; c++) {
      (function (cb) {
        cb.addEventListener('change', function () {
          syncGlobalCheck();
          syncGroupCheck(cb.getAttribute('data-group'));
        });
      })(importChecks[c]);
    }
  }

  /* ---------- 侧栏标签页切换 ---------- */

  var VALID_TABS = ['overview', 'categories', 'links'];
  var DEFAULT_TAB = 'overview';

  var tabItems = document.querySelectorAll('.sidebar-item[data-nav]');
  var tabPanels = document.querySelectorAll('.tab-panel[data-panel]');

  /* ---------- 汉堡菜单（移动端抽屉） ---------- */
  var hamburgerBtn = byId('hamburger-btn');
  var sidebarEl = byId('admin-sidebar');
  var sidebarMask = byId('sidebar-mask');

  function openSidebar() {
    if (!sidebarEl) return;
    sidebarEl.classList.add('open');
    if (sidebarMask) sidebarMask.hidden = false;
    if (hamburgerBtn) hamburgerBtn.setAttribute('aria-expanded', 'true');
  }
  function closeSidebar() {
    if (!sidebarEl) return;
    sidebarEl.classList.remove('open');
    if (sidebarMask) sidebarMask.hidden = true;
    if (hamburgerBtn) hamburgerBtn.setAttribute('aria-expanded', 'false');
  }

  if (hamburgerBtn) hamburgerBtn.addEventListener('click', function () {
    if (sidebarEl && sidebarEl.classList.contains('open')) closeSidebar();
    else openSidebar();
  });
  if (sidebarMask) sidebarMask.addEventListener('click', closeSidebar);

  var sidebarCloseBtn = byId('sidebar-close');
  if (sidebarCloseBtn) sidebarCloseBtn.addEventListener('click', closeSidebar);

  /* 点击侧栏任意链接后自动收起抽屉 */
  if (sidebarEl) {
    sidebarEl.addEventListener('click', function (e) {
      if (e.target.closest('a')) closeSidebar();
    });
  }

  /* 屏幕放大到桌面端时，确保抽屉状态复位 */
  var mql = window.matchMedia('(min-width: 901px)');
  var onWide = function () { if (mql.matches) closeSidebar(); };
  if (mql.addEventListener) mql.addEventListener('change', onWide);
  else if (mql.addListener) mql.addListener(onWide);

  /* ---------- 设置子菜单展开/收起 ---------- */
  var submenuBtns = document.querySelectorAll('.sidebar-item[data-submenu]');
  for (var s = 0; s < submenuBtns.length; s++) {
    (function (btn) {
      var panel = document.querySelector(
        '[data-submenu-panel="' + btn.getAttribute('data-submenu') + '"]');
      btn.addEventListener('click', function () {
        var open = panel && panel.classList.toggle('open');
        btn.setAttribute('aria-expanded', open ? 'true' : 'false');
      });
    })(submenuBtns[s]);
  }

  /* 初始同步子菜单按钮的 aria-expanded 与展开状态 */
  for (var s2 = 0; s2 < submenuBtns.length; s2++) {
    (function (btn) {
      var panel = document.querySelector(
        '[data-submenu-panel="' + btn.getAttribute('data-submenu') + '"]');
      var open = panel && panel.classList.contains('open');
      btn.setAttribute('aria-expanded', open ? 'true' : 'false');
    })(submenuBtns[s2]);
  }

  function switchTab(name) {
    /* 只接受已知标签，避免 URL hash 被滥用 */
    if (VALID_TABS.indexOf(name) === -1) name = DEFAULT_TAB;

    for (var i = 0; i < tabPanels.length; i++) {
      tabPanels[i].hidden = tabPanels[i].getAttribute('data-panel') !== name;
    }
    for (var j = 0; j < tabItems.length; j++) {
      var on = tabItems[j].getAttribute('data-nav') === name;
      tabItems[j].classList.toggle('active', on);
      tabItems[j].setAttribute('aria-current', on ? 'page' : 'false');
    }
  }

  /* 只有含 tab 面板的页面（后台概览/分类/网址）才启用标签切换 */
  var hasTabs = tabPanels.length > 0;
  if (hasTabs) {
    for (var t = 0; t < tabItems.length; t++) {
      (function (btn) {
        btn.addEventListener('click', function () {
          var name = btn.getAttribute('data-nav');
          switchTab(name);
          /* 写入 hash，刷新后仍停留在当前标签 */
          if (window.history && window.history.replaceState) {
            window.history.replaceState(null, '', '#' + name);
          } else {
            window.location.hash = name;
          }
        });
      })(tabItems[t]);
    }

    /* 概览页的快捷跳转按钮 */
    var gotos = document.querySelectorAll('[data-goto]');
    for (var g = 0; g < gotos.length; g++) {
      (function (btn) {
        btn.addEventListener('click', function () {
          var name = btn.getAttribute('data-goto');
          switchTab(name);
          if (window.history && window.history.replaceState) {
            window.history.replaceState(null, '', '#' + name);
          } else {
            window.location.hash = name;
          }
        });
      })(gotos[g]);
    }

    /* 支持 hash 变化（浏览器前进后退、手动改地址） */
    window.addEventListener('hashchange', function () {
      switchTab(window.location.hash.replace('#', ''));
    });

    /* 初始标签：优先取 URL hash */
    switchTab(window.location.hash.replace('#', '') || DEFAULT_TAB);
  }

  function openDialog(id) {
    var d = byId(id);
    if (!d) return;
    /* 每次打开都回到内容顶部，避免沿用上次的滚动位置 */
    d.scrollTop = 0;
    if (typeof d.showModal === 'function') d.showModal();
    else d.setAttribute('open', '');
  }

  function closeDialog(id) {
    var d = byId(id);
    if (!d) return;
    if (typeof d.close === 'function') d.close();
    else d.removeAttribute('open');
  }

  /* 关闭按钮 */
  var closers = document.querySelectorAll('[data-close]');
  for (var i = 0; i < closers.length; i++) {
    (function (btn) {
      btn.addEventListener('click', function () { closeDialog(btn.getAttribute('data-close')); });
    })(closers[i]);
  }

  /* 点击遮罩关闭 */
  var dialogs = document.querySelectorAll('dialog.dialog');
  for (var j = 0; j < dialogs.length; j++) {
    (function (d) {
      d.addEventListener('click', function (e) {
        if (e.target === d) closeDialog(d.id);
      });
    })(dialogs[j]);
  }

  /* ---------- 分类：新增 / 编辑 ---------- */

  function resetCatForm() {
    var form = byId('cat-form');
    if (!form) return;
    form.action = ADMIN_BASE + '/categories';
    form.reset();
    byId('cat-dialog-title').textContent = '新增分类';
  }

  var catAddBtn = document.querySelector('[data-open="cat-dialog"]');
  if (catAddBtn) {
    catAddBtn.addEventListener('click', function () {
      resetCatForm();
      openDialog('cat-dialog');
    });
  }

  var catEditBtns = document.querySelectorAll('[data-edit-cat]');
  for (var k = 0; k < catEditBtns.length; k++) {
    (function (btn) {
      btn.addEventListener('click', function () {
        var row = btn.closest('tr');
        if (!row) return;
        byId('cat-form').action = ADMIN_BASE + '/categories/' + row.getAttribute('data-id');
        byId('cat-dialog-title').textContent = '编辑分类';
        byId('cat-name').value = row.getAttribute('data-name') || '';
        byId('cat-icon').value = row.getAttribute('data-icon') || '';
        byId('cat-sort').value = row.getAttribute('data-sort') || '';
        openDialog('cat-dialog');
      });
    })(catEditBtns[k]);
  }

  /* ---------- 网址：新增 / 编辑 ---------- */

  function resetLinkForm() {
    var form = byId('link-form');
    if (!form) return;
    form.action = ADMIN_BASE + '/links';
    form.reset();
    byId('link-dialog-title').textContent = '新增网址';
  }

  var linkAddBtn = document.querySelector('[data-open="link-dialog"]');
  if (linkAddBtn) {
    linkAddBtn.addEventListener('click', function () {
      resetLinkForm();
      openDialog('link-dialog');
    });
  }

  var linkEditBtns = document.querySelectorAll('[data-edit-link]');
  for (var m = 0; m < linkEditBtns.length; m++) {
    (function (btn) {
      btn.addEventListener('click', function () {
        var row = btn.closest('tr');
        if (!row) return;
        byId('link-form').action = ADMIN_BASE + '/links/' + row.getAttribute('data-id');
        byId('link-dialog-title').textContent = '编辑网址';
        byId('link-cat').value = row.getAttribute('data-cat') || '';
        byId('link-title').value = row.getAttribute('data-title') || '';
        byId('link-url').value = row.getAttribute('data-url') || '';
        byId('link-icon').value = row.getAttribute('data-icon') || '';
        byId('link-desc').value = row.getAttribute('data-desc') || '';
        byId('link-sort').value = row.getAttribute('data-sort') || '';
        openDialog('link-dialog');
      });
    })(linkEditBtns[m]);
  }

  /* ---------- 网址：多选删除 ---------- */
  var linkChecks = document.querySelectorAll('.link-check');
  var linkCheckAll = document.querySelector('.link-check-all');
  var batchSelectAll = byId('link-select-all');
  var batchDelete = byId('link-batch-delete');
  var batchIds = byId('link-batch-ids');
  var batchForm = byId('link-batch-form');

  function refreshBatchState() {
    var checked = [];
    for (var i = 0; i < linkChecks.length; i++) {
      if (linkChecks[i].checked) checked.push(linkChecks[i].value);
    }
    if (batchIds) batchIds.value = checked.join(',');
    if (batchDelete) batchDelete.disabled = checked.length === 0;
    var all = checked.length > 0 && checked.length === linkChecks.length;
    if (linkCheckAll) linkCheckAll.checked = all;
    if (batchSelectAll) batchSelectAll.checked = all;
  }

  function setAllChecks(checked) {
    for (var i = 0; i < linkChecks.length; i++) {
      linkChecks[i].checked = checked;
    }
    refreshBatchState();
  }

  if (linkCheckAll) {
    linkCheckAll.addEventListener('change', function () {
      setAllChecks(linkCheckAll.checked);
    });
  }
  if (batchSelectAll) {
    batchSelectAll.addEventListener('change', function () {
      setAllChecks(batchSelectAll.checked);
    });
  }
  for (var n = 0; n < linkChecks.length; n++) {
    (function (cb) {
      cb.addEventListener('change', refreshBatchState);
    })(linkChecks[n]);
  }

  if (batchForm) {
    batchForm.addEventListener('submit', function (e) {
      var checked = [];
      for (var i = 0; i < linkChecks.length; i++) {
        if (linkChecks[i].checked) checked.push(linkChecks[i].value);
      }
      if (checked.length === 0) {
        e.preventDefault();
        return;
      }
      if (!confirm('确定删除选中的 ' + checked.length + ' 条网址？')) {
        e.preventDefault();
      }
    });
  }
})();
