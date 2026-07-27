/**
 * Google Analytics Optimizer
 * - Lazy load: Load GA after user interaction
 * - Reading time tracking
 * - Scroll depth tracking
 * - Outbound link tracking
 * - File download tracking
 */
(function() {
  'use strict';

  var GA_ID = window.GA_MEASUREMENT_ID;
  var CONFIG = window.GA_CONFIG || {};
  var gaLoaded = false;
  var eventQueue = [];

  // === Utility Functions ===

  function sendEvent(name, params) {
    if (gaLoaded && typeof gtag === 'function') {
      gtag('event', name, params);
    } else {
      eventQueue.push({ name: name, params: params });
    }
  }

  // 暴露 sendEvent 供其他模块使用
  window.gaSendEvent = sendEvent;

  function flushEventQueue() {
    eventQueue.forEach(function(event) {
      gtag('event', event.name, event.params);
    });
    eventQueue = [];
  }

  // === Lazy Loading Logic ===

  var triggerEvents = ['scroll', 'click', 'keydown', 'touchstart', 'mousemove'];

  function loadGA() {
    if (gaLoaded) return;
    gaLoaded = true;
    window.gaLoaded = true;

    // Remove all listeners
    triggerEvents.forEach(function(event) {
      window.removeEventListener(event, loadGA, { passive: true });
    });

    // Dynamically create and load gtag.js
    var script = document.createElement('script');
    script.async = true;
    script.src = 'https://www.googletagmanager.com/gtag/js?id=' + GA_ID;
    script.onload = function() {
      // Initialize gtag
      window.dataLayer = window.dataLayer || [];
      window.gtag = function() { dataLayer.push(arguments); };
      gtag('js', new Date());
      gtag('config', GA_ID, {
        send_page_view: true,
        cookie_flags: 'SameSite=None;Secure'
      });

      // Send queued events
      flushEventQueue();

      // Initialize all trackers
      initTrackers();
    };
    document.head.appendChild(script);
  }

  function initTriggers() {
    triggerEvents.forEach(function(event) {
      window.addEventListener(event, loadGA, { once: true, passive: true });
    });

    // Fallback: auto-load after timeout
    var timeout = CONFIG.lazyLoadTimeout || 3000;
    setTimeout(function() {
      if (!gaLoaded) loadGA();
    }, timeout);
  }

  // === Reading Time Tracker ===

  function initReadingTimeTracker() {
    var startTime = Date.now();
    var totalTime = 0;
    var isVisible = true;
    var milestones = [30, 60, 180, 300, 600]; // seconds
    var sentMilestones = {};

    document.addEventListener('visibilitychange', function() {
      if (document.hidden) {
        totalTime += (Date.now() - startTime) / 1000;
        isVisible = false;
      } else {
        startTime = Date.now();
        isVisible = true;
      }
    });

    setInterval(function() {
      if (!isVisible) return;

      var currentTime = totalTime + (Date.now() - startTime) / 1000;

      milestones.forEach(function(milestone) {
        if (currentTime >= milestone && !sentMilestones[milestone]) {
          sentMilestones[milestone] = true;
          sendEvent('reading_time', {
            event_category: 'engagement',
            event_label: formatTime(milestone),
            value: milestone,
            page_path: window.location.pathname
          });
        }
      });
    }, 5000);

    window.addEventListener('beforeunload', function() {
      var finalTime = Math.round(totalTime + (isVisible ? (Date.now() - startTime) / 1000 : 0));
      if (finalTime > 0 && gaLoaded && typeof gtag === 'function') {
        gtag('event', 'reading_time_total', {
          event_category: 'engagement',
          value: finalTime,
          page_path: window.location.pathname,
          transport_type: 'beacon'
        });
      }
    });
  }

  function formatTime(seconds) {
    if (seconds < 60) return seconds + 's';
    return Math.floor(seconds / 60) + 'm';
  }

  // === Scroll Depth Tracker ===

  function initScrollDepthTracker() {
    var milestones = [25, 50, 75, 100];
    var sentMilestones = {};
    var ticking = false;

    function getScrollPercent() {
      var h = document.documentElement;
      var b = document.body;
      var scrollTop = h.scrollTop || b.scrollTop;
      var scrollHeight = (h.scrollHeight || b.scrollHeight) - h.clientHeight;

      if (scrollHeight <= 0) return 100;
      return Math.round((scrollTop / scrollHeight) * 100);
    }

    function checkScrollDepth() {
      var percent = getScrollPercent();

      milestones.forEach(function(milestone) {
        if (percent >= milestone && !sentMilestones[milestone]) {
          sentMilestones[milestone] = true;
          sendEvent('scroll_depth', {
            event_category: 'engagement',
            event_label: milestone + '%',
            value: milestone,
            page_path: window.location.pathname
          });
        }
      });

      ticking = false;
    }

    window.addEventListener('scroll', function() {
      if (!ticking) {
        requestAnimationFrame(checkScrollDepth);
        ticking = true;
      }
    }, { passive: true });
  }

  // === Outbound Link Tracker ===

  function initOutboundLinkTracker() {
    var currentHost = window.location.hostname;

    document.addEventListener('click', function(e) {
      var link = e.target.closest('a');
      if (!link) return;

      var href = link.href;
      if (!href) return;

      try {
        var url = new URL(href);

        // Skip internal links, anchor links, javascript, mailto, tel
        if (url.hostname === currentHost ||
            url.protocol === 'javascript:' ||
            url.protocol === 'mailto:' ||
            url.protocol === 'tel:') {
          return;
        }

        sendEvent('outbound_link', {
          event_category: 'outbound',
          event_label: href,
          link_text: (link.textContent || '').trim().substring(0, 100),
          link_domain: url.hostname,
          page_path: window.location.pathname
        });
      } catch (err) {
        // Invalid URL, ignore
      }
    }, { passive: true });
  }

  // === File Download Tracker ===

  function initDownloadTracker() {
    var downloadExtensions = [
      'pdf', 'zip', 'rar', '7z', 'tar', 'gz',
      'doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx',
      'mp3', 'mp4', 'wav', 'avi', 'mov',
      'exe', 'dmg', 'apk'
    ];

    var extensionRegex = new RegExp('\\.(' + downloadExtensions.join('|') + ')($|\\?)', 'i');

    document.addEventListener('click', function(e) {
      var link = e.target.closest('a');
      if (!link || !link.href) return;

      var href = link.href;
      var match = href.match(extensionRegex);

      if (match) {
        var extension = match[1].toLowerCase();
        var filename = href.split('/').pop().split('?')[0];

        sendEvent('file_download', {
          event_category: 'download',
          event_label: filename,
          file_extension: extension,
          file_name: filename,
          link_url: href,
          page_path: window.location.pathname
        });
      }
    }, { passive: true });
  }

  // === Initialize All Trackers ===

  function initTrackers() {
    if (CONFIG.enableReadingTime !== false) initReadingTimeTracker();
    if (CONFIG.enableScrollDepth !== false) initScrollDepthTracker();
    if (CONFIG.enableOutboundLinks !== false) initOutboundLinkTracker();
    if (CONFIG.enableDownloads !== false) initDownloadTracker();
  }

  // === Start ===

  if (GA_ID) {
    initTriggers();
  }

})();
