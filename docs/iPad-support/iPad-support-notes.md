Got the lay of the land. The core constraint is hard: iPadOS Safari has no
  Pointer Lock API and no way for web code to warp the system cursor, so we     
  can't reproduce desktop-style infinite mouse-look — the OS cursor will always 
  pin at the screen edge and stop emitting deltas. <Pointer>/delta from Unity's 
  Input System is just reading that pinned delta.                               
                                                                                
  The realistic fix is drag-to-look on iPad: user presses the trackpad button   
  (or taps-and-holds) and drags to rotate the camera. We'd detect iPad at       
  runtime via a tiny jslib that checks the user-agent, and on iPad swap the Look
   behavior:                                                                    

  - On pointerdown over the canvas: capture the pointer and start tracking.     
  - On pointermove while captured: accumulate movementX/movementY into a delta  
  buffer that the C# side drains each frame (same shape the Look action already 
  expects).                                                                     
  - On pointerup / cancel: stop.                                                
  - CSS-hide the cursor during a drag so it feels like look-mode rather than    
  cursor-mode.                                                                  
  - Keyboard (WASD, tools, etc.) stays exactly as it is.                        
                                                                                
  This mirrors what Roblox, Sketchfab, and most WebGL 3D apps do on iPad.       
  Tradeoff: look and click-to-interact now share the pointer button, so we      
  either (a) gate interact on a short press vs. drag (most common), or (b) make 
  interact a keyboard-only action on iPad (simplest and fine since this is the
  keyboard config).