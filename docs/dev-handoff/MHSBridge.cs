using System.Runtime.InteropServices;
using UnityEngine;

/// <summary>
/// Bridge between the Unity game and the StrataHub PWA host page.
/// Attach this script to a GameObject named "MHSBridge" in the first scene that loads.
///
/// Two operating modes:
///   PWA mode  - StrataHub manages unit transitions. The game signals completion
///               and the host page handles navigation, progress tracking, and downloads.
///   URL mode  - The game manages its own navigation using relative URLs.
///               A loader page determines the current unit from save data and navigates to it.
///
/// Mode is determined automatically: if the host page calls SendMessage('MHSBridge', 'OnPWAReady', ''),
/// the game is in PWA mode. Otherwise it's in URL mode.
///
/// Player identity comes from the URL parameter ?id=login_id in both modes.
/// </summary>
public class MHSBridge : MonoBehaviour
{
    // --- JS function imports (from MHSBridge.jslib) ---

    [DllImport("__Internal")]
    private static extern void MHSBridge_NotifyUnitComplete(string unitId);

    [DllImport("__Internal")]
    private static extern string MHSBridge_GetPlayerID();

    [DllImport("__Internal")]
    private static extern void MHSBridge_NavigateToUnit(string url);

    // --- State ---

    private bool _isPWA = false;

    /// <summary>True if the game is running inside the StrataHub PWA.</summary>
    public bool IsPWA => _isPWA;

    // --- Singleton ---

    public static MHSBridge Instance { get; private set; }

    private void Awake()
    {
        if (Instance != null && Instance != this) { Destroy(gameObject); return; }
        Instance = this;
        DontDestroyOnLoad(gameObject);
    }

    // --- Called by the PWA host page via SendMessage ---

    /// <summary>
    /// Called by the PWA host page after Unity finishes loading.
    /// Signals that the game is running inside the managed PWA environment.
    /// The game should NOT navigate between units when in PWA mode.
    /// </summary>
    public void OnPWAReady(string unused)
    {
        _isPWA = true;
        Debug.Log("MHSBridge: PWA mode activated");
    }

    // --- Called by game code ---

    /// <summary>
    /// Call this when the current unit is complete.
    /// In PWA mode: notifies the host page, which handles the transition.
    /// In URL mode: navigates to the next unit using a relative URL.
    /// </summary>
    /// <param name="currentUnitId">The unit that was just completed, e.g., "unit3"</param>
    /// <param name="nextUnitRelativeUrl">
    /// Relative URL to the next unit, e.g., "../unit4/index.html".
    /// Ignored in PWA mode. Pass null or empty for the final unit in URL mode.
    /// </param>
    public void CompleteUnit(string currentUnitId, string nextUnitRelativeUrl)
    {
#if UNITY_WEBGL && !UNITY_EDITOR
        if (_isPWA)
        {
            // PWA mode: notify the host page. It handles the transition.
            MHSBridge_NotifyUnitComplete(currentUnitId);
        }
        else
        {
            // URL mode: navigate to the next unit directly.
            if (!string.IsNullOrEmpty(nextUnitRelativeUrl))
            {
                MHSBridge_NavigateToUnit(nextUnitRelativeUrl);
            }
        }
#endif
    }

    /// <summary>
    /// Returns the player's login ID from the URL parameter ?id=value.
    /// Works in both PWA and URL modes.
    /// Use this instead of calling /api/user.
    /// </summary>
    public string GetPlayerID()
    {
#if UNITY_WEBGL && !UNITY_EDITOR
        return MHSBridge_GetPlayerID();
#else
        return "editor-test-user";
#endif
    }

    /// <summary>
    /// Navigates to a unit by name (e.g., "unit1", "unit3").
    /// Used by the loader to send the student to their current unit.
    /// URL parameters (?id=...) are preserved automatically.
    /// No-ops in PWA mode — StrataHub handles navigation directly.
    /// </summary>
    public void NavigateToUnit(string unitName)
    {
#if UNITY_WEBGL && !UNITY_EDITOR
        if (_isPWA)
        {
            Debug.LogWarning("MHSBridge: NavigateToUnit ignored in PWA mode");
            return;
        }
        MHSBridge_NavigateToUnit("../" + unitName + "/index.html");
#endif
    }
}
