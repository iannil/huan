/**
 * Google Analytics 事件追踪桥接模块
 * 提供统一 API ZrsAnalytics.track() 发送自定义事件
 * 支持会话级身份、设备检测、来源分类
 * 调试模式：URL 参数 ?ga_debug=1 或控制台 ZrsAnalytics.debug.enable()
 */
(function() {
  'use strict';

  // === 调试模式 ===
  var DEBUG = (function() {
    var enabled = false;
    var storageKey = 'zrs_ga_debug';

    // 检查 URL 参数或 localStorage
    function init() {
      if (window.location.search.indexOf('ga_debug=1') !== -1) {
        enabled = true;
        try { localStorage.setItem(storageKey, '1'); } catch(e) {}
      } else {
        try { enabled = localStorage.getItem(storageKey) === '1'; } catch(e) {}
      }
    }

    init();

    return {
      isEnabled: function() { return enabled; },
      enable: function() {
        enabled = true;
        try { localStorage.setItem(storageKey, '1'); } catch(e) {}
        console.log('[ZrsAnalytics] Debug mode enabled');
      },
      disable: function() {
        enabled = false;
        try { localStorage.removeItem(storageKey); } catch(e) {}
        console.log('[ZrsAnalytics] Debug mode disabled');
      },
      log: function(eventName, params) {
        if (!enabled) return;
        console.log('%c[ZrsAnalytics]%c ' + eventName, 'color: #4CAF50; font-weight: bold;', 'color: inherit;', params);
      }
    };
  })();

  // === 会话身份管理 ===
  var SessionIdentity = {
    sessionKey: 'zrs_session_id',

    // 生成简单唯一 ID
    generateId: function() {
      return Date.now().toString(36) + Math.random().toString(36).substr(2, 9);
    },

    // 获取或创建会话 ID
    getSessionId: function() {
      var sessionId = null;
      try {
        sessionId = sessionStorage.getItem(this.sessionKey);
        if (!sessionId) {
          sessionId = this.generateId();
          sessionStorage.setItem(this.sessionKey, sessionId);
        }
      } catch(e) {
        sessionId = this.generateId();
      }
      return sessionId;
    },

    // 检测设备类型
    getDeviceType: function() {
      var ua = navigator.userAgent;
      if (/tablet|ipad|playbook|silk/i.test(ua)) return 'tablet';
      if (/mobile|iphone|ipod|android|blackberry|opera mini|iemobile/i.test(ua)) return 'mobile';
      return 'desktop';
    },

    // 分类来源类型
    getReferrerType: function() {
      var referrer = document.referrer;
      if (!referrer) return 'direct';

      try {
        var url = new URL(referrer);
        var hostname = url.hostname.toLowerCase();

        // 同域名
        if (hostname === window.location.hostname) return 'internal';

        // 搜索引擎
        var searchEngines = ['google', 'bing', 'yahoo', 'baidu', 'sogou', 'so.com', 'duckduckgo'];
        for (var i = 0; i < searchEngines.length; i++) {
          if (hostname.indexOf(searchEngines[i]) !== -1) return 'search';
        }

        // 社交平台
        var socialPlatforms = ['weibo', 'weixin', 'wechat', 'qq.com', 'douyin', 'tiktok', 'facebook', 'twitter', 'instagram', 'linkedin'];
        for (var j = 0; j < socialPlatforms.length; j++) {
          if (hostname.indexOf(socialPlatforms[j]) !== -1) return 'social';
        }

        return 'referral';
      } catch(e) {
        return 'unknown';
      }
    },

    // 获取内容类型
    getContentType: function() {
      var path = window.location.pathname;
      if (path.indexOf('/books/') !== -1) return 'book';
      if (path.indexOf('/practices/') !== -1) return 'practice';
      if (path.indexOf('/posts/') !== -1) return 'post';
      if (path.indexOf('/gallery/') !== -1) return 'gallery';
      return 'page';
    }
  };

  // === 通用参数 ===
  var commonParams = null;

  function getCommonParams() {
    if (!commonParams) {
      commonParams = {
        session_id: SessionIdentity.getSessionId(),
        device_type: SessionIdentity.getDeviceType(),
        referrer_type: SessionIdentity.getReferrerType()
      };
    }
    return {
      session_id: commonParams.session_id,
      device_type: commonParams.device_type,
      referrer_type: commonParams.referrer_type,
      page_path: window.location.pathname,
      content_type: SessionIdentity.getContentType()
    };
  }

  // === 事件发送 ===
  function track(eventName, params) {
    params = params || {};

    // 合并通用参数
    var eventParams = getCommonParams();
    for (var key in params) {
      if (params.hasOwnProperty(key)) {
        eventParams[key] = params[key];
      }
    }

    // 调试日志
    DEBUG.log(eventName, eventParams);

    // 发送事件
    if (typeof window.gaSendEvent === 'function') {
      window.gaSendEvent(eventName, eventParams);
    } else if (typeof window.gtag === 'function') {
      window.gtag('event', eventName, eventParams);
    }
  }

  // === 导航点击追踪 ===
  function initNavTracking() {
    document.addEventListener('click', function(e) {
      var link = e.target.closest('a');
      if (!link) return;

      // 导航菜单点击
      if (link.closest('.nav') || link.closest('.navbar') || link.closest('.menu')) {
        var navText = (link.textContent || '').trim().substring(0, 50);
        var navHref = link.getAttribute('href') || '';

        // 排除搜索等非导航元素
        if (navHref && navHref !== '#' && !link.classList.contains('search-icon-btn')) {
          track('zrs_nav_click', {
            nav_text: navText,
            nav_href: navHref
          });
        }
      }

      // 社交图标点击
      if (link.closest('.social') || link.closest('.social-icons') || link.classList.contains('social-icon')) {
        var href = link.getAttribute('href') || '';
        var platform = 'unknown';

        // 识别社交平台
        if (href.indexOf('github') !== -1) platform = 'github';
        else if (href.indexOf('twitter') !== -1 || href.indexOf('x.com') !== -1) platform = 'twitter';
        else if (href.indexOf('weibo') !== -1) platform = 'weibo';
        else if (href.indexOf('zhihu') !== -1) platform = 'zhihu';
        else if (href.indexOf('bilibili') !== -1) platform = 'bilibili';
        else if (href.indexOf('youtube') !== -1) platform = 'youtube';
        else if (href.indexOf('mailto:') !== -1) platform = 'email';
        else if (href.indexOf('rss') !== -1 || href.indexOf('feed') !== -1) platform = 'rss';

        track('zrs_social_click', {
          social_platform: platform,
          social_href: href.substring(0, 100)
        });
      }
    }, { passive: true });
  }

  // === 初始化 ===
  function init() {
    initNavTracking();

    // 调试模式提示
    if (DEBUG.isEnabled()) {
      console.log('[ZrsAnalytics] Initialized with debug mode');
      console.log('[ZrsAnalytics] Session ID:', SessionIdentity.getSessionId());
      console.log('[ZrsAnalytics] Device:', SessionIdentity.getDeviceType());
      console.log('[ZrsAnalytics] Referrer:', SessionIdentity.getReferrerType());
    }
  }

  // DOM 就绪后初始化
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }

  // === 导出 API ===
  window.ZrsAnalytics = {
    track: track,
    debug: DEBUG,
    getSessionId: function() { return SessionIdentity.getSessionId(); },
    getDeviceType: function() { return SessionIdentity.getDeviceType(); },
    getReferrerType: function() { return SessionIdentity.getReferrerType(); },
    getContentType: function() { return SessionIdentity.getContentType(); }
  };

})();
