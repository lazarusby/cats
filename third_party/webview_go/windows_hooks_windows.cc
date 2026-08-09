#define WEBVIEW_HEADER
#include "_cgo_export.h"
#include "libs/webview/include/webview.h"
#include "libs/mswebview2/include/WebView2.h"

#include <atomic>
#include <cstdint>
#include <new>
#include <shellapi.h>
#include <windows.h>

namespace {

static constexpr IID iid_iunknown{0x00000000, 0x0000, 0x0000,
                                  {0xC0, 0x00, 0x00, 0x00,
                                   0x00, 0x00, 0x00, 0x46}};

template <typename Interface, const IID *InterfaceID> class EventHandler : public Interface {
public:
  explicit EventHandler(uintptr_t go_handle) : go_handle_(go_handle) {}

  HRESULT STDMETHODCALLTYPE QueryInterface(REFIID iid, void **object) override {
    if (!object) {
      return E_POINTER;
    }
    if (IsEqualIID(iid, iid_iunknown) || IsEqualIID(iid, *InterfaceID)) {
      *object = static_cast<Interface *>(this);
      AddRef();
      return S_OK;
    }
    *object = nullptr;
    return E_NOINTERFACE;
  }

  ULONG STDMETHODCALLTYPE AddRef() override { return ++references_; }

  ULONG STDMETHODCALLTYPE Release() override {
    const auto references = --references_;
    if (references == 0) {
      delete this;
    }
    return references;
  }

protected:
  uintptr_t go_handle_;

private:
  std::atomic<ULONG> references_{1};
};

class NavigationHandler final
    : public EventHandler<ICoreWebView2NavigationStartingEventHandler,
                          &IID_ICoreWebView2NavigationStartingEventHandler> {
public:
  using EventHandler::EventHandler;

  HRESULT STDMETHODCALLTYPE Invoke(ICoreWebView2 *,
                                   ICoreWebView2NavigationStartingEventArgs *args) override {
    if (!args) {
      return E_POINTER;
    }
    LPWSTR uri = nullptr;
    const auto result = args->get_Uri(&uri);
    if (FAILED(result)) {
      return result;
    }
    const int action = goWebviewWindowsNavigation(
        go_handle_, reinterpret_cast<uint16_t *>(uri), 0);
    if (action != 0) {
      args->put_Cancel(TRUE);
    }
    if (action == 2 && uri && *uri) {
      ShellExecuteW(nullptr, L"open", uri, nullptr, nullptr, SW_SHOWNORMAL);
    }
    CoTaskMemFree(uri);
    return S_OK;
  }
};

class NewWindowHandler final
    : public EventHandler<ICoreWebView2NewWindowRequestedEventHandler,
                          &IID_ICoreWebView2NewWindowRequestedEventHandler> {
public:
  using EventHandler::EventHandler;

  HRESULT STDMETHODCALLTYPE Invoke(ICoreWebView2 *,
                                   ICoreWebView2NewWindowRequestedEventArgs *args) override {
    if (!args) {
      return E_POINTER;
    }
    LPWSTR uri = nullptr;
    const auto result = args->get_Uri(&uri);
    if (FAILED(result)) {
      return result;
    }
    const int action = goWebviewWindowsNavigation(
        go_handle_, reinterpret_cast<uint16_t *>(uri), 1);
    args->put_Handled(TRUE);
    if (action == 2 && uri && *uri) {
      ShellExecuteW(nullptr, L"open", uri, nullptr, nullptr, SW_SHOWNORMAL);
    }
    CoTaskMemFree(uri);
    return S_OK;
  }
};

class AcceleratorHandler final
    : public EventHandler<ICoreWebView2AcceleratorKeyPressedEventHandler,
                          &IID_ICoreWebView2AcceleratorKeyPressedEventHandler> {
public:
  using EventHandler::EventHandler;

  HRESULT STDMETHODCALLTYPE Invoke(ICoreWebView2Controller *,
                                   ICoreWebView2AcceleratorKeyPressedEventArgs *args) override {
    if (!args) {
      return E_POINTER;
    }
    COREWEBVIEW2_KEY_EVENT_KIND kind{};
    UINT virtual_key = 0;
    if (FAILED(args->get_KeyEventKind(&kind)) ||
        FAILED(args->get_VirtualKey(&virtual_key))) {
      return E_FAIL;
    }
    uint32_t modifiers = 0;
    if (GetKeyState(VK_SHIFT) < 0)
      modifiers |= 1;
    if (GetKeyState(VK_CONTROL) < 0)
      modifiers |= 2;
    if (GetKeyState(VK_MENU) < 0)
      modifiers |= 4;
    if (GetKeyState(VK_LWIN) < 0 || GetKeyState(VK_RWIN) < 0)
      modifiers |= 8;
    if (GetKeyState(VK_RMENU) < 0 && GetKeyState(VK_CONTROL) < 0)
      modifiers |= 16;
    if (goWebviewWindowsAccelerator(go_handle_, virtual_key,
                                    static_cast<uint32_t>(kind), modifiers)) {
      args->put_Handled(TRUE);
    }
    return S_OK;
  }
};

struct WindowsHooks {
  ICoreWebView2 *webview{};
  ICoreWebView2Controller *controller{};
  EventRegistrationToken navigation_token{};
  EventRegistrationToken new_window_token{};
  EventRegistrationToken accelerator_token{};
  bool navigation_registered{};
  bool new_window_registered{};
  bool accelerator_registered{};
};

void remove_hooks(WindowsHooks *hooks) {
  if (!hooks)
    return;
  if (hooks->webview && hooks->navigation_registered)
    hooks->webview->remove_NavigationStarting(hooks->navigation_token);
  if (hooks->webview && hooks->new_window_registered)
    hooks->webview->remove_NewWindowRequested(hooks->new_window_token);
  if (hooks->controller && hooks->accelerator_registered)
    hooks->controller->remove_AcceleratorKeyPressed(hooks->accelerator_token);
  if (hooks->webview)
    hooks->webview->Release();
  if (hooks->controller)
    hooks->controller->Release();
  delete hooks;
}

} // namespace

extern "C" void *webview_install_windows_hooks(webview_t instance,
                                                uintptr_t go_handle,
                                                int *error_code) {
  if (error_code)
    *error_code = E_FAIL;
  auto *controller = static_cast<ICoreWebView2Controller *>(
      webview_get_native_handle(instance,
                                WEBVIEW_NATIVE_HANDLE_KIND_BROWSER_CONTROLLER));
  if (!controller) {
    if (error_code)
      *error_code = E_POINTER;
    return nullptr;
  }
  auto *hooks = new (std::nothrow) WindowsHooks{};
  if (!hooks) {
    if (error_code)
      *error_code = E_OUTOFMEMORY;
    return nullptr;
  }
  hooks->controller = controller;
  controller->AddRef();
  HRESULT result = controller->get_CoreWebView2(&hooks->webview);
  if (FAILED(result) || !hooks->webview) {
    if (error_code)
      *error_code = result;
    remove_hooks(hooks);
    return nullptr;
  }

  auto *navigation = new (std::nothrow) NavigationHandler(go_handle);
  auto *new_window = new (std::nothrow) NewWindowHandler(go_handle);
  auto *accelerator = new (std::nothrow) AcceleratorHandler(go_handle);
  if (!navigation || !new_window || !accelerator) {
    if (navigation)
      navigation->Release();
    if (new_window)
      new_window->Release();
    if (accelerator)
      accelerator->Release();
    if (error_code)
      *error_code = E_OUTOFMEMORY;
    remove_hooks(hooks);
    return nullptr;
  }

  result = hooks->webview->add_NavigationStarting(navigation,
                                                  &hooks->navigation_token);
  navigation->Release();
  if (FAILED(result)) {
    new_window->Release();
    accelerator->Release();
    if (error_code)
      *error_code = result;
    remove_hooks(hooks);
    return nullptr;
  }
  hooks->navigation_registered = true;

  result = hooks->webview->add_NewWindowRequested(new_window,
                                                  &hooks->new_window_token);
  new_window->Release();
  if (FAILED(result)) {
    accelerator->Release();
    if (error_code)
      *error_code = result;
    remove_hooks(hooks);
    return nullptr;
  }
  hooks->new_window_registered = true;

  result = controller->add_AcceleratorKeyPressed(accelerator,
                                                  &hooks->accelerator_token);
  accelerator->Release();
  if (FAILED(result)) {
    if (error_code)
      *error_code = result;
    remove_hooks(hooks);
    return nullptr;
  }
  hooks->accelerator_registered = true;
  if (error_code)
    *error_code = S_OK;
  return hooks;
}

extern "C" void webview_remove_windows_hooks(void *raw_hooks) {
  remove_hooks(static_cast<WindowsHooks *>(raw_hooks));
}
