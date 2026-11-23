import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Link, Route, Routes } from 'react-router-dom'
import Layout from './components/Layout'
import { AuthProvider } from './lib/auth'
import { CartProvider } from './lib/cart'
import { AuthPage, RequireAuth } from './pages/Auth'
import Cart from './pages/Cart'
import Checkout from './pages/Checkout'
import Home from './pages/Home'
import Orders, { OrderDetail } from './pages/Orders'
import Product from './pages/Product'
import Store from './pages/Store'
import './styles.css'

function NotFound() {
  return (
    <div className="container empty">
      <h2>Page not found</h2>
      <Link to="/" className="btn btn-primary">
        Go home
      </Link>
    </div>
  )
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
      <AuthProvider>
        <CartProvider>
          <Routes>
            <Route path="/login" element={<AuthPage mode="login" />} />
            <Route path="/register" element={<AuthPage mode="register" />} />
            <Route element={<Layout />}>
              <Route index element={<Home />} />
              <Route path="store" element={<Store />} />
              <Route path="product/:slug" element={<Product />} />
              <Route path="cart" element={<Cart />} />
              <Route path="checkout" element={<RequireAuth><Checkout /></RequireAuth>} />
              <Route path="orders" element={<RequireAuth><Orders /></RequireAuth>} />
              <Route path="orders/:id" element={<RequireAuth><OrderDetail /></RequireAuth>} />
              <Route path="*" element={<NotFound />} />
            </Route>
          </Routes>
        </CartProvider>
      </AuthProvider>
    </BrowserRouter>
  </StrictMode>,
)
