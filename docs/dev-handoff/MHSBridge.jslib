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
  // Returns a pointer to a C string. Returns empty string if not found.
  //
  // Memory: _malloc'd buffer is freed by Unity's IL2CPP string marshaller.
  // When the C# return type is string, IL2CPP copies the UTF8 data and
  // calls _free on the returned pointer. This is Unity's documented pattern
  // for returning strings from jslib plugins.
  MHSBridge_GetPlayerID: function() {
    var params = new URLSearchParams(window.location.search);
    var id = params.get('id') || '';
    var bufferSize = lengthBytesUTF8(id) + 1;
    var buffer = _malloc(bufferSize);
    stringToUTF8(id, buffer, bufferSize);
    return buffer;
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
