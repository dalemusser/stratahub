mergeInto(LibraryManager.library, {

  // ---------------------------------------------------------------------------
  // iPadInput JavaScript Plugin (jslib)
  //
  // Provides a two-finger trackpad look fallback for iPad, where Safari has no
  // Pointer Lock API and the OS cursor pins at screen edges. A `wheel` event
  // listener on the canvas captures two-finger trackpad drags (which fire
  // wheel events independent of cursor position) and accumulates the deltas
  // for the C# side to drain each frame.
  //
  // On non-iPad devices, iPadInput_Init() returns 0 and the C# side falls back
  // to the normal Pointer/delta look path. Desktop behavior is untouched.
  // ---------------------------------------------------------------------------

  $iPadInputState: {
    initialized: false,
    isIPad: false,
    deltaX: 0,
    deltaY: 0
  },

  iPadInput_Init__deps: ['$iPadInputState'],
  iPadInput_Init: function() {
    console.log('iPadInput.jslib: iPadInput_Init called');
    if (iPadInputState.initialized) {
      console.log('iPadInput.jslib: already initialized, isIPad=' + iPadInputState.isIPad);
      return iPadInputState.isIPad ? 1 : 0;
    }
    iPadInputState.initialized = true;

    // iPadOS 13+ reports platform "MacIntel" with touch support; also match
    // the legacy "iPad" UA string.
    var ua = navigator.userAgent || '';
    var platform = navigator.platform || '';
    var touchPoints = (typeof navigator.maxTouchPoints === 'number') ? navigator.maxTouchPoints : 0;
    var isIPad = /iPad/.test(ua) || (platform === 'MacIntel' && touchPoints > 1);
    iPadInputState.isIPad = isIPad;
    console.log('iPadInput.jslib: detection — platform=' + platform + ', touchPoints=' + touchPoints + ', isIPad=' + isIPad);
    if (!isIPad) return 0;

    var target = (typeof Module !== 'undefined' && Module && Module.canvas)
      ? Module.canvas
      : document.querySelector('canvas');
    if (!target) target = window;
    console.log('iPadInput.jslib: attaching wheel listener to ' + (target === window ? 'window' : (target.tagName || 'element')));

    target.addEventListener('wheel', function(e) {
      iPadInputState.deltaX += e.deltaX || 0;
      iPadInputState.deltaY += e.deltaY || 0;
      if (e.cancelable) e.preventDefault();
    }, { passive: false });

    return 1;
  },

  iPadInput_DrainDeltaX__deps: ['$iPadInputState'],
  iPadInput_DrainDeltaX: function() {
    var v = iPadInputState.deltaX;
    iPadInputState.deltaX = 0;
    return v;
  },

  iPadInput_DrainDeltaY__deps: ['$iPadInputState'],
  iPadInput_DrainDeltaY: function() {
    var v = iPadInputState.deltaY;
    iPadInputState.deltaY = 0;
    return v;
  }

});
