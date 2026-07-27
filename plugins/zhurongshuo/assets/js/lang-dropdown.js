'use strict';

(function () {
  document.addEventListener('DOMContentLoaded', function () {
    var dropdown = document.getElementById('lang_dropdown');
    if (!dropdown) return;
    var trigger = dropdown.querySelector('.lang-dropdown-trigger');
    if (!trigger) return;

    function close() {
      dropdown.classList.remove('open');
      trigger.setAttribute('aria-expanded', 'false');
    }

    function open() {
      dropdown.classList.add('open');
      trigger.setAttribute('aria-expanded', 'true');
    }

    trigger.addEventListener('click', function (e) {
      e.stopPropagation();
      if (dropdown.classList.contains('open')) {
        close();
      } else {
        open();
      }
    });

    document.addEventListener('click', function (e) {
      if (!dropdown.contains(e.target)) close();
    });

    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') close();
    });

    window.addEventListener('blur', close);
  });
})();
