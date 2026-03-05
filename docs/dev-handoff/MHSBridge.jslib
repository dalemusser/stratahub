mergeInto(LibraryManager.library, {

  // Called by C# to notify the PWA that a unit has been completed.
  // unitIdPtr: pointer to a C string containing the unit ID (e.g., "unit3")
  MHSBridge_NotifyUnitComplete: function(unitIdPtr) {
    var unitId = UTF8ToString(unitIdPtr);
    if (typeof window.mhsUnitComplete === 'function') {
      window.mhsUnitComplete(unitId);
    }
  },

  // Called by C# to get the player's login ID from the URL parameters.
  // Returns a pointer to a _malloc'd UTF-8 C string (null-terminated).
  // Caller MUST free the returned pointer via MHSBridge_Free().
  MHSBridge_GetPlayerID: function() {
    var params = new URLSearchParams(window.location.search);
    var id = params.get('id') || '';
    var bufferSize = lengthBytesUTF8(id) + 1;
    var buffer = _malloc(bufferSize);
    stringToUTF8(id, buffer, bufferSize);
    return buffer;
  },

  // Called by C# to free any _malloc'd pointer returned by this plugin.
  MHSBridge_Free: function(ptr) {
    if (ptr) {
      _free(ptr);
    }
  },

  // Called by C# to navigate to a unit using a relative URL.
  // Resolves the relative URL against the current page and carries all
  // URL parameters (?id=, ?group=, etc.) forward to the destination.
  MHSBridge_NavigateToUnit: function(nextUnitPtr) {
    var nextUnit = UTF8ToString(nextUnitPtr);
    // Resolve relative URL against current page, then merge current params
    var url = new URL(nextUnit, window.location.href);
    var currentParams = new URLSearchParams(window.location.search);
    currentParams.forEach(function(value, key) {
      url.searchParams.set(key, value);
    });
    window.location.href = url.href;
  }

});
