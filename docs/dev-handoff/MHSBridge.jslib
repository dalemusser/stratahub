mergeInto(LibraryManager.library, {

  // Called by C# to notify the PWA that a unit has been completed.
  // unitIdPtr: pointer to a C string containing the unit ID (e.g., "unit3")
  MHSBridge_NotifyUnitComplete: function(unitIdPtr) {
    var unitId = UTF8ToString(unitIdPtr);
    if (typeof window.mhsUnitComplete === 'function') {
      window.mhsUnitComplete(unitId);
    }
  },

  // Called by C# to check if the game is running inside a PWA.
  // Returns 1 (true) if OnPWAReady was called, 0 (false) otherwise.
  MHSBridge_IsPWA: function() {
    return window.__mhsIsPWA ? 1 : 0;
  },

  // Called by C# to get the player's login ID from the URL parameters.
  // Returns a pointer to a C string. Returns empty string if not found.
  MHSBridge_GetPlayerID: function() {
    var params = new URLSearchParams(window.location.search);
    var id = params.get('id') || '';
    var bufferSize = lengthBytesUTF8(id) + 1;
    var buffer = _malloc(bufferSize);
    stringToUTF8(id, buffer, bufferSize);
    return buffer;
  },

  // Called by C# to navigate to the next unit (Mode 2 only).
  // nextUnitPtr: pointer to a C string containing the relative URL (e.g., "../unit2/index.html")
  MHSBridge_NavigateToUnit: function(nextUnitPtr) {
    var nextUnit = UTF8ToString(nextUnitPtr);
    // Carry URL parameters (identity) through to the next unit
    var search = window.location.search;
    window.location.href = nextUnit + search;
  }

});
