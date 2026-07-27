/**
 * 全局搜索模块
 * 使用 Fuse.js 实现客户端模糊搜索
 * 快捷键: Ctrl/Cmd + K
 */
(function() {
  'use strict';

  const CONFIG = {
    searchIndexUrl: '/search.json',
    fuseOptions: {
      keys: [
        { name: 'title', weight: 2.0 },
        { name: 'content', weight: 1.0 },
        { name: 'tags', weight: 1.5 }
      ],
      includeScore: true,
      includeMatches: true,
      threshold: 0.4,
      ignoreLocation: true,
      minMatchCharLength: 1
    },
    debounceDelay: 300,
    maxResults: 20
  };

  let searchIndex = [];
  let fuse = null;
  let selectedIndex = -1;
  let currentResults = [];
  let isLoadingIndex = false;

  // DOM 元素
  const elements = {
    dialog: null,
    input: null,
    closeBtn: null,
    results: null,
    status: null
  };

  /**
   * 初始化 DOM 元素引用
   */
  function initElements() {
    elements.dialog = document.getElementById('search-dialog');
    elements.input = document.getElementById('search-input');
    elements.closeBtn = document.getElementById('search-close');
    elements.results = document.getElementById('search-results');
    elements.status = document.getElementById('search-status');
  }

  /**
   * 加载搜索索引
   */
  async function loadSearchIndex() {
    if (isLoadingIndex) return;
    if (searchIndex.length > 0) return searchIndex;

    isLoadingIndex = true;
    showStatus('loading', '正在加载搜索索引...');

    try {
      const response = await fetch(CONFIG.searchIndexUrl);
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }
      searchIndex = await response.json();
      fuse = new Fuse(searchIndex, CONFIG.fuseOptions);
      hideStatus();
      return searchIndex;
    } catch (error) {
      console.error('Failed to load search index:', error);
      showStatus('error', '搜索索引加载失败，请刷新页面重试');
      return null;
    } finally {
      isLoadingIndex = false;
    }
  }

  /**
   * 显示状态消息
   */
  function showStatus(type, message) {
    if (!elements.status) return;
    elements.status.className = `search-status ${type}`;
    elements.status.textContent = message;
    elements.status.style.display = 'block';
  }

  /**
   * 隐藏状态消息
   */
  function hideStatus() {
    if (!elements.status) return;
    elements.status.style.display = 'none';
  }

  /**
   * 高亮匹配文本
   */
  function highlightMatches(text, matches) {
    if (!matches || matches.length === 0) return escapeHtml(text);

    // 收集所有匹配位置
    const allMatches = [];
    matches.forEach(match => {
      if (match.indices) {
        match.indices.forEach((indice) => {
          allMatches.push({ start: indice[0], end: indice[1] + 1 });
        });
      }
    });

    // 按起始位置排序
    allMatches.sort((a, b) => a.start - b.start);

    // 合并重叠的匹配
    const merged = [];
    for (const match of allMatches) {
      if (merged.length === 0 || match.start > merged[merged.length - 1].end) {
        merged.push({ ...match });
      } else {
        merged[merged.length - 1].end = Math.max(merged[merged.length - 1].end, match.end);
      }
    }

    // 构建高亮字符串
    let result = '';
    let lastIndex = 0;
    for (const match of merged) {
      result += escapeHtml(text.substring(lastIndex, match.start));
      result += '<span class="search-highlight">' + escapeHtml(text.substring(match.start, match.end)) + '</span>';
      lastIndex = match.end;
    }
    result += escapeHtml(text.substring(lastIndex));

    return result;
  }

  /**
   * HTML 转义
   */
  function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }

  /**
   * 获取内容类型标签
   */
  function getTypeLabel(url) {
    if (url.includes('/posts/')) return { label: '文章', class: 'type-post' };
    if (url.includes('/books/')) return { label: '读书', class: 'type-book' };
    if (url.includes('/practices/')) return { label: '实践', class: 'type-practice' };
    return { label: '页面', class: 'type-page' };
  }

  /**
   * 渲染搜索结果
   */
  function renderResults(results) {
    if (!elements.results) return;

    if (results.length === 0) {
      elements.results.innerHTML = `
        <div class="search-empty">
          <div class="search-empty-icon">🔍</div>
          <div class="search-empty-text">未找到匹配结果</div>
        </div>
      `;
      return;
    }

    const html = results.slice(0, CONFIG.maxResults).map((result, index) => {
      const item = result.item;
      const typeInfo = getTypeLabel(item.url);

      // 高亮标题
      let titleHtml = escapeHtml(item.title);
      const titleMatch = result.matches?.find(m => m.key === 'title');
      if (titleMatch) {
        titleHtml = highlightMatches(item.title, [titleMatch]);
      }

      // 高亮内容
      let contentHtml = escapeHtml(item.content);
      const contentMatch = result.matches?.find(m => m.key === 'content');
      if (contentMatch) {
        // 只显示匹配部分的上下文
        const indices = contentMatch.indices || [];
        if (indices.length > 0) {
          const firstMatch = indices[0];
          const start = Math.max(0, firstMatch[0] - 50);
          const end = Math.min(item.content.length, firstMatch[1] + 51);
          let preview = item.content.substring(start, end);
          if (start > 0) preview = '...' + preview;
          if (end < item.content.length) preview = preview + '...';
          contentHtml = highlightMatches(preview, [contentMatch]);
        }
      }

      // 标签
      let tagsHtml = '';
      if (item.tags && item.tags.length > 0) {
        tagsHtml = '<span class="result-tags">' +
          item.tags.map(tag => `<span class="result-tag">${escapeHtml(tag)}</span>`).join('') +
          '</span>';
      }

      // 日期
      let dateHtml = '';
      if (item.date) {
        dateHtml = `<span class="result-date">${item.date}</span>`;
      }

      return `
        <div class="search-result-item" data-index="${index}" data-url="${escapeHtml(item.url)}">
          <div class="search-result-title">
            <span class="result-type">${typeInfo.label}</span>
            <span>${titleHtml}</span>
          </div>
          <div class="search-result-content">${contentHtml}</div>
          <div class="search-result-meta">
            ${dateHtml}
            ${tagsHtml}
          </div>
        </div>
      `;
    }).join('');

    elements.results.innerHTML = html;
    currentResults = results;
    selectedIndex = -1;

    // 绑定点击事件
    elements.results.querySelectorAll('.search-result-item').forEach(item => {
      item.addEventListener('click', function() {
        const url = this.dataset.url;
        const index = parseInt(this.dataset.index, 10);
        if (url) {
          // 追踪搜索结果点击
          if (window.ZrsAnalytics) {
            ZrsAnalytics.track('zrs_search_click', {
              result_url: url,
              result_position: index + 1,
              click_method: 'mouse'
            });
          }
          window.location.href = url;
        }
      });
    });
  }

  /**
   * 执行搜索
   */
  async function performSearch(query) {
    if (!fuse) {
      const index = await loadSearchIndex();
      if (!index) return;
    }

    if (!query.trim()) {
      elements.results.innerHTML = '';
      hideStatus();
      currentResults = [];
      selectedIndex = -1;
      return;
    }

    const results = fuse.search(query);

    // 追踪搜索查询事件
    if (window.ZrsAnalytics) {
      ZrsAnalytics.track('zrs_search_query', {
        query_text: query.substring(0, 100),
        result_count: results.length
      });
    }

    renderResults(results);
  }

  /**
   * 防抖函数
   */
  function debounce(func, delay) {
    let timeoutId;
    return function(...args) {
      clearTimeout(timeoutId);
      timeoutId = setTimeout(() => func.apply(this, args), delay);
    };
  }

  /**
   * 打开搜索对话框
   */
  async function openSearchDialog(trigger) {
    trigger = trigger || 'unknown';

    if (!elements.dialog) {
      initElements();
    }

    // 追踪搜索打开事件
    if (window.ZrsAnalytics) {
      ZrsAnalytics.track('zrs_search_open', {
        trigger: trigger
      });
    }

    elements.dialog.style.display = 'flex';
    elements.input.value = '';
    elements.results.innerHTML = '';
    hideStatus();
    selectedIndex = -1;
    currentResults = [];

    // 预加载索引
    if (searchIndex.length === 0) {
      await loadSearchIndex();
    }

    // 聚焦输入框
    setTimeout(() => {
      elements.input.focus();
    }, 100);

    document.body.style.overflow = 'hidden';
  }

  /**
   * 关闭搜索对话框
   */
  function closeSearchDialog() {
    if (!elements.dialog) return;
    elements.dialog.style.display = 'none';
    document.body.style.overflow = '';
  }

  /**
   * 处理键盘导航
   */
  function handleKeyNavigation(e) {
    const items = elements.results.querySelectorAll('.search-result-item');

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      selectedIndex = Math.min(selectedIndex + 1, items.length - 1);
      updateSelection(items);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      selectedIndex = Math.max(selectedIndex - 1, -1);
      updateSelection(items);
    } else if (e.key === 'Enter') {
      if (selectedIndex >= 0 && items[selectedIndex]) {
        e.preventDefault();
        const url = items[selectedIndex].dataset.url;
        if (url) {
          // 追踪搜索结果点击（键盘）
          if (window.ZrsAnalytics) {
            ZrsAnalytics.track('zrs_search_click', {
              result_url: url,
              result_position: selectedIndex + 1,
              click_method: 'keyboard'
            });
          }
          window.location.href = url;
        }
      }
    } else if (e.key === 'Escape') {
      closeSearchDialog();
    }
  }

  /**
   * 更新选中状态
   */
  function updateSelection(items) {
    items.forEach((item, index) => {
      if (index === selectedIndex) {
        item.classList.add('selected');
        item.scrollIntoView({ block: 'nearest' });
      } else {
        item.classList.remove('selected');
      }
    });
  }

  /**
   * 设置事件监听
   */
  function setupEventListeners() {
    // 全局快捷键
    document.addEventListener('keydown', function(e) {
      // Ctrl/Cmd + K
      if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
        e.preventDefault();
        openSearchDialog('keyboard');
      }
    });

    // 搜索输入（带防抖）
    const debouncedSearch = debounce(function(query) {
      performSearch(query);
    }, CONFIG.debounceDelay);

    elements.input.addEventListener('input', function(e) {
      debouncedSearch(e.target.value);
    });

    // 键盘导航
    elements.input.addEventListener('keydown', handleKeyNavigation);

    // 关闭按钮
    elements.closeBtn.addEventListener('click', closeSearchDialog);

    // 点击外部关闭
    elements.dialog.addEventListener('click', function(e) {
      if (e.target === elements.dialog) {
        closeSearchDialog();
      }
    });

    // 搜索图标按钮（旧版）
    const searchIconBtn = document.querySelector('.search-icon-btn');
    if (searchIconBtn) {
      searchIconBtn.addEventListener('click', function(e) {
        e.preventDefault();
        openSearchDialog('icon');
      });
    }

    // 导航栏搜索输入框（点击打开搜索对话框）
    const navSearchInput = document.getElementById('nav-search-input');
    if (navSearchInput) {
      navSearchInput.addEventListener('click', function(e) {
        e.preventDefault();
        openSearchDialog('nav_input');
      });
      navSearchInput.addEventListener('keydown', function(e) {
        if (e.key === 'Enter') {
          e.preventDefault();
          openSearchDialog('nav_input');
        }
      });
    }
  }

  /**
   * 初始化
   */
  function init() {
    initElements();
    if (elements.dialog) {
      setupEventListeners();
    }
  }

  // DOM 就绪后初始化
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }

  // 导出 API
  window.GlobalSearch = {
    open: openSearchDialog,
    close: closeSearchDialog,
    search: performSearch
  };
})();
